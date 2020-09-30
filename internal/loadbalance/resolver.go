package loadbalance

import (
	"context"
	"google.golang.org/grpc/attributes"
	"log"
	api "mafia/log/api/v1"

	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"
)

// Scheme identifier. When you call grpc.Dial ,
// gRPC parses out the scheme from the target address you gave it and tries
// to find a resolver that matches, defaulting to its DNS resolver.
// For our resolver, you’ll format the target address like this:
// <Name>://your-service-address.
const Name = "proglog"

var _ resolver.Builder = (*Resolver)(nil)
var _ resolver.Resolver = (*Resolver)(nil)

type Resolver struct {
	mu            sync.Mutex
	clientConn    resolver.ClientConn
	resolverConn  *grpc.ClientConn
	serviceConfig *serviceconfig.ParseResult
}

func init() {
	resolver.Register(&Resolver{})
}

func (r *Resolver) ResolveNow(_ resolver.ResolveNowOptions) {
	r.mu.Lock()
	defer r.mu.Unlock()

	client := api.NewLogClient(r.resolverConn)
	// get cluster and then set on cc attributes
	ctx := context.Background()
	res, err := client.GetServers(ctx, &api.GetServersRequest{})
	if err != nil {
		// TODO logger here
		log.Printf(
			"[ERROR] %s: failed to resolve servers: %v",
			Name,
			err,
		)
		return
	}

	var addresses []resolver.Address
	for _, server := range res.Servers {
		addresses = append(addresses, resolver.Address{
			Addr: server.RpcAddr,
			Attributes: attributes.New(
				"is_leader",
				server.IsLeader,
			),
		})
	}

	r.clientConn.UpdateState(resolver.State{
		Addresses:     addresses,
		ServiceConfig: r.serviceConfig,
	})
}

func (r *Resolver) Close() {
	if err := r.resolverConn.Close(); err != nil {
		// TODO logger here
		log.Printf(
			"[ERROR] %s: failed to close conn: %v",
			Name,
			err,
		)
	}
}

func (r *Resolver) Scheme() string {
	return Name
}

func (r *Resolver) Build(
	target resolver.Target,
	cc resolver.ClientConn,
	opts resolver.BuildOptions,
) (resolver.Resolver, error) {
	r.clientConn = cc
	var dialOpts []grpc.DialOption

	if opts.DialCreds != nil {
		dialOpts = append(
			dialOpts,
			grpc.WithTransportCredentials(opts.DialCreds),
		)
	}

	r.serviceConfig = r.clientConn.ParseServiceConfig(
		fmt.Sprintf(`{"loadBalancingConfig":[{"%s":{}}]}`, Name),
	)

	var err error
	r.resolverConn, err = grpc.Dial(target.Endpoint, dialOpts...)
	if err != nil {
		return nil, err
	}

	r.ResolveNow(resolver.ResolveNowOptions{})

	return r, nil
}
