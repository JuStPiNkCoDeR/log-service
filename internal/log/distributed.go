package log

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"io"
	api "mafia/log/api/v1"
	"net"

	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

type DistributedLog struct {
	config Config
	log    *Log
	raft   *raft.Raft
}

func NewDistributedLog(dataDir string, config Config) (
	*DistributedLog,
	error,
) {
	l := &DistributedLog{
		config: config,
	}

	if err := l.setupLog(dataDir); err != nil {
		return nil, err
	}

	if err := l.setupRaft(dataDir); err != nil {
		return nil, err
	}

	return l, nil
}

// Creates the log for this server.
func (l *DistributedLog) setupLog(dataDir string) (err error) {
	logDir := filepath.Join(dataDir, "log")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	l.log, err = NewLog(logDir, l.config)

	return
}

// Configures and creates the server’s Raft instance.
func (l *DistributedLog) setupRaft(dataDir string) error {
	fsm := &fsm{log: l.log}

	logDir := filepath.Join(dataDir, "raft", "log")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logConfig := l.config
	logConfig.Segment.InitialOffset = 1
	logStore, err := newLogStore(logDir, logConfig)
	if err != nil {
		return err
	}

	stableStore, err := raftboltdb.NewBoltStore(
		filepath.Join(dataDir, "raft", "stable"),
	)
	if err != nil {
		return err
	}

	retain := 1
	snapshotStore, err := raft.NewFileSnapshotStore(
		filepath.Join(dataDir, "raft"),
		retain,
		os.Stderr,
	)
	if err != nil {
		return err
	}

	maxPool := 5
	timeout := 10 * time.Second
	transport := raft.NewNetworkTransport(
		l.config.Raft.StreamLayer,
		maxPool,
		timeout,
		os.Stderr,
	)

	config := raft.DefaultConfig()
	config.LocalID = l.config.Raft.LocalID

	if l.config.Raft.HeartbeatTimeout != 0 {
		config.HeartbeatTimeout = l.config.Raft.HeartbeatTimeout
	}

	if l.config.Raft.ElectionTimeout != 0 {
		config.ElectionTimeout = l.config.Raft.ElectionTimeout
	}

	if l.config.Raft.LeaderLeaseTimeout != 0 {
		config.LeaderLeaseTimeout = l.config.Raft.LeaderLeaseTimeout
	}

	if l.config.Raft.CommitTimeout != 0 {
		config.CommitTimeout = l.config.Raft.CommitTimeout
	}

	l.raft, err = raft.NewRaft(
		config,
		fsm,
		logStore,
		stableStore,
		snapshotStore,
		transport,
	)
	if err != nil {
		return err
	}

	if l.config.Raft.Bootstrap {
		config := raft.Configuration{
			Servers: []raft.Server{{
				ID:      config.LocalID,
				Address: transport.LocalAddr(),
			}},
		}
		err = l.raft.BootstrapCluster(config).Error()
	}

	return err
}

// Wraps Raft's API to apply requests and return their responses.
func (l *DistributedLog) apply(reqType RequestType, req proto.Marshaler) (
	interface{},
	error,
) {
	var buff bytes.Buffer

	_, err := buff.Write([]byte{byte(reqType)})
	if err != nil {
		return nil, err
	}

	b, err := req.Marshal()
	if err != nil {
		return nil, err
	}

	_, err = buff.Write(b)
	if err != nil {
		return nil, err
	}

	timeout := 10 * time.Second
	future := l.raft.Apply(buff.Bytes(), timeout)
	if future.Error() != nil {
		return nil, future.Error()
	}

	res := future.Response()
	if err, ok := res.(error); ok {
		return nil, err
	}

	return res, nil
}

// Wraps Append method on own Log API
func (l *DistributedLog) Append(record *api.Record) (uint64, error) {
	res, err := l.apply(
		AppendRequestType,
		&api.ProduceRequest{Record: record},
	)
	if err != nil {
		return 0, err
	}

	return res.(*api.ProduceResponse).Offset, nil
}

// Reads the record for the offset from server's log.
func (l *DistributedLog) Read(offset uint64) (
	*api.Record,
	error,
) {
	return l.log.Read(offset)
}

// Adds the server to the Raft cluster.
func (l *DistributedLog) Join(id, addr string) error {
	configFuture := l.raft.GetConfiguration()

	if err := configFuture.Error(); err != nil {
		return err
	}

	serverID := raft.ServerID(id)
	serverAddr := raft.ServerAddress(addr)

	for _, srv := range configFuture.Configuration().Servers {
		if srv.ID == serverID || srv.Address == serverAddr {
			if srv.ID == serverID && srv.Address == serverAddr {
				// server has already joined
				return nil
			}
			// remove the existing server
			removeFuture := l.raft.RemoveServer(serverID, 0, 0)

			if err := removeFuture.Error(); err != nil {
				return err
			}
		}
	}

	addFuture := l.raft.AddVoter(serverID, serverAddr, 0, 0)

	if err := addFuture.Error(); err != nil {
		return err
	}

	return nil
}

// Removes the server from the cluster.
func (l *DistributedLog) Leave(id string) error {
	removeFuture := l.raft.RemoveServer(raft.ServerID(id), 0, 0)

	return removeFuture.Error()
}

// Blocks until the cluster has elected a leader or times out.
func (l *DistributedLog) WaitForLeader(timeout time.Duration) error {
	timeoutClose := time.After(timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutClose:
			return fmt.Errorf("timed out")
		case <-ticker.C:
			if l := l.raft.Leader(); l != "" {
				return nil
			}
		}
	}
}

// Shuts down the Raft instance and close the local Log
func (l *DistributedLog) Close() error {
	f := l.raft.Shutdown()

	if err := f.Error(); err != nil {
		return err
	}

	return l.log.Close()
}

func (l *DistributedLog) GetServers() (servers []*api.Server, err error) {
	future := l.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, err
	}

	for _, server := range future.Configuration().Servers {
		servers = append(servers, &api.Server{
			Id:       string(server.ID),
			RpcAddr:  string(server.Address),
			IsLeader: l.raft.Leader() == server.Address,
		})
	}

	return
}

// Force compiler to check for interface implementation.
var _ raft.FSM = (*fsm)(nil)
var _ raft.FSMSnapshot = (*snapshot)(nil)
var _ raft.LogStore = (*logStore)(nil)
var _ raft.StreamLayer = (*StreamLayer)(nil)

type RequestType uint8

const (
	AppendRequestType RequestType = 0
)

type fsm struct {
	log *Log
}

func (m *fsm) applyAppend(b []byte) interface{} {
	var req api.ProduceRequest

	err := req.Unmarshal(b)
	if err != nil {
		return err
	}

	offset, err := m.log.Append(req.Record)
	if err != nil {
		return err
	}

	return &api.ProduceResponse{Offset: offset}
}

func (m *fsm) Apply(record *raft.Log) interface{} {
	buff := record.Data
	reqType := RequestType(buff[0])

	switch reqType {
	case AppendRequestType:
		return m.applyAppend(buff[1:])
	}

	return nil
}

func (m *fsm) Snapshot() (raft.FSMSnapshot, error) {
	r := m.log.Reader()

	return &snapshot{reader: r}, nil
}

func (m *fsm) Restore(closer io.ReadCloser) error {
	if err := m.log.Reset(); err != nil {
		return err
	}

	b := make([]byte, lenWidth)
	var buff bytes.Buffer

	for {
		_, err := io.ReadFull(closer, b)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		size := int64(enc.Uint64(b))
		if _, err = io.CopyN(&buff, closer, size); err != nil {
			return err
		}

		record := &api.Record{}
		if err = record.Unmarshal(buff.Bytes()); err != nil {
			return err
		}

		if _, err = m.log.Append(record); err != nil {
			return err
		}

		buff.Reset()
	}

	return nil
}

type snapshot struct {
	reader io.Reader
}

func (s *snapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := io.Copy(sink, s.reader); err != nil {
		_ = sink.Cancel()
		return err
	}

	return sink.Close()
}

func (s *snapshot) Release() {}

type logStore struct {
	*Log
}

func newLogStore(dir string, c Config) (*logStore, error) {
	log, err := NewLog(dir, c)
	if err != nil {
		return nil, err
	}

	return &logStore{log}, nil
}

func (l logStore) FirstIndex() (uint64, error) {
	return l.LowestOffset()
}

func (l logStore) LastIndex() (uint64, error) {
	return l.HighestOffset()
}

func (l logStore) GetLog(index uint64, log *raft.Log) error {
	in, err := l.Read(index)
	if err != nil {
		return err
	}

	log.Data = in.Value
	log.Index = in.Offset
	log.Type = raft.LogType(in.Type)
	log.Term = in.Term

	return nil
}

func (l logStore) StoreLog(log *raft.Log) error {
	return l.StoreLogs([]*raft.Log{log})
}

func (l logStore) StoreLogs(records []*raft.Log) error {
	for _, record := range records {
		if _, err := l.Append(&api.Record{
			Value: record.Data,
			Term:  record.Term,
			Type:  uint32(record.Type),
		}); err != nil {
			return err
		}
	}

	return nil
}

func (l logStore) DeleteRange(_, max uint64) error {
	return l.Truncate(max)
}

type StreamLayer struct {
	ln              net.Listener
	serverTLSConfig *tls.Config
	peerTLSConfig   *tls.Config
}

func NewStreamLayer(
	ln net.Listener,
	serverTLSConfig,
	peerTLSConfig *tls.Config,
) *StreamLayer {
	return &StreamLayer{
		ln:              ln,
		serverTLSConfig: serverTLSConfig,
		peerTLSConfig:   peerTLSConfig,
	}
}

const RaftRPC = 1

func (s *StreamLayer) Dial(
	addr raft.ServerAddress,
	timeout time.Duration,
) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", string(addr))
	if err != nil {
		return nil, err
	}
	// identify to mux this is a raft rpc
	_, err = conn.Write([]byte{byte(RaftRPC)})
	if err != nil {
		return nil, err
	}

	if s.peerTLSConfig != nil {
		conn = tls.Client(conn, s.peerTLSConfig)
	}

	return conn, err
}

func (s *StreamLayer) Accept() (net.Conn, error) {
	conn, err := s.ln.Accept()
	if err != nil {
		return nil, err
	}

	b := make([]byte, 1)
	_, err = conn.Read(b)
	if err != nil {
		return nil, err
	}

	// TODO logger here
	if !bytes.Equal([]byte{byte(RaftRPC)}, b) {
		return nil, fmt.Errorf("not a raft rpc")
	}

	if s.serverTLSConfig != nil {
		return tls.Server(conn, s.serverTLSConfig), nil
	}

	return conn, nil
}

func (s *StreamLayer) Close() error {
	return s.ln.Close()
}

func (s *StreamLayer) Addr() net.Addr {
	return s.ln.Addr()
}
