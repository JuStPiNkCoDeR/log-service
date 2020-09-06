package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/travisjeffery/go-dynaport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"io/ioutil"
	api "mafia/log/api/v1"
	"mafia/log/internal/config"
	"os"
	"testing"
	"time"
)

func client(
	t *testing.T,
	agent *Agent,
	tlsConfig *tls.Config,
) api.LogClient {
	tlsCredits := credentials.NewTLS(tlsConfig)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(tlsCredits)}
	rpcAddr, err := agent.Config.RPCAddr()
	require.NoError(t, err)

	conn, err := grpc.Dial(rpcAddr, opts...)
	require.NoError(t, err)

	client := api.NewLogClient(conn)

	return client
}

func TestAgent(t *testing.T) {
	// Setup TLS configurations
	serverTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CertFile:      config.ServerCertFile,
		KeyFile:       config.ServerKeyFile,
		CAFile:        config.CAFile,
		Server:        true,
		ServerAddress: "127.0.0.1",
	})
	require.NoError(t, err)

	peerTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CertFile:      config.RootClientCertFile,
		KeyFile:       config.RootClientKeyFile,
		CAFile:        config.CAFile,
		Server:        false,
		ServerAddress: "127.0.0.1",
	})
	require.NoError(t, err)

	// Setup the cluster
	var agents []*Agent

	for i := 0; i < 3; i++ {
		ports := dynaport.Get(2)
		bindPort := fmt.Sprintf("%s:%d", "127.0.0.1", ports[0])
		rpcPort := ports[1]

		dataDir, err := ioutil.TempDir("", "server-test-log")
		require.NoError(t, err)

		var startJoinAddresses []string

		if i != 0 {
			startJoinAddresses = append(
				startJoinAddresses,
				agents[0].Config.BindAddr,
			)
		}

		agent, err := New(Config{
			NodeName:           fmt.Sprintf("%d", i),
			StartJoinAddresses: startJoinAddresses,
			BindAddr:           bindPort,
			RPCPort:            rpcPort,
			DataDir:            dataDir,
			ACLPolicyFile:      config.ACLPolicyFile,
			ACLModelFile:       config.ACLModelFile,
			ServerTLSConfig:    serverTLSConfig,
			PeerTLSConfig:      peerTLSConfig,
			Bootstrap:          i == 0,
		})
		require.NoError(t, err)

		agents = append(agents, agent)
	}
	defer func() {
		for _, agent := range agents {
			err := agent.Shutdown()
			require.NoError(t, err)
			require.NoError(t,
				os.RemoveAll(agent.Config.DataDir),
			)
		}
	}()
	time.Sleep(3 * time.Second)

	// Test work
	leaderClient := client(t, agents[0], peerTLSConfig)
	produceResponse, err := leaderClient.Produce(
		context.Background(),
		&api.ProduceRequest{
			Record: &api.Record{
				Value: []byte("foo"),
			},
		},
	)
	require.NoError(t, err)

	consumeResponse, err := leaderClient.Consume(
		context.Background(),
		&api.ConsumeRequest{
			Offset: produceResponse.Offset,
		},
	)
	require.NoError(t, err)

	require.Equal(t, consumeResponse.Record.Value, []byte("foo"))

	// wait until replication has finished
	time.Sleep(3 * time.Second)

	followerClient := client(t, agents[1], peerTLSConfig)
	consumeResponse, err = followerClient.Consume(
		context.Background(),
		&api.ConsumeRequest{
			Offset: produceResponse.Offset,
		},
	)
	require.NoError(t, err)

	require.Equal(t, consumeResponse.Record.Value, []byte("foo"))

	consumeResponse, err = leaderClient.Consume(
		context.Background(),
		&api.ConsumeRequest{
			Offset: produceResponse.Offset + 1,
		},
	)
	require.Nil(t, consumeResponse)
	require.Error(t, err)

	got := status.Code(err)
	want := status.Code(api.ErrOffsetOutOfRange{}.GRPCStatus().Err())
	require.Equal(t, got, want)
}
