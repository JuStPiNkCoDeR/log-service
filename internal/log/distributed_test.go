package log

//func TestMultiNodes(t *testing.T) {
//	var logs []*DistributedLog
//	nodeCount := 2
//	ports := dynaport.Get(nodeCount)
//
//	// Setup cluster
//	for i := 0; i < nodeCount; i++ {
//		dataDir, err := ioutil.TempDir("", "distributed-log-test")
//		require.NoError(t, err)
//		defer func(dir string) {
//			err = os.RemoveAll(dir)
//		}(dataDir)
//
//		ln, err := net.Listen(
//			"tcp",
//			fmt.Sprintf("127.0.0.1:%d", ports[i]),
//		)
//		require.NoError(t, err)
//
//		config := Config{}
//		config.Raft.StreamLayer = NewStreamLayer(ln, nil, nil)
//		config.Raft.LocalID = raft.ServerID(fmt.Sprintf("%d", i))
//		config.Raft.HeartbeatTimeout = 50 * time.Millisecond
//		config.Raft.ElectionTimeout = 50 * time.Millisecond
//		config.Raft.LeaderLeaseTimeout = 50 * time.Millisecond
//		config.Raft.CommitTimeout = 7 * time.Millisecond
//
//		if i == 0 {
//			config.Raft.Bootstrap = true
//		}
//
//		l, err := NewDistributedLog(dataDir, config)
//		require.NoError(t, err)
//
//		// TODO solve some sh@!t at this place
//		// when it comes to add the third node the first one is not a leader
//		// WTF?!
//		if i != 0 {
//			err = logs[0].Join(
//				fmt.Sprintf("%d", i), ln.Addr().String(),
//			)
//			require.NoError(t, err)
//		} else {
//			err = l.WaitForLeader(3 * time.Second)
//			require.NoError(t, err)
//		}
//
//		logs = append(logs, l)
//	}
//
//	// Proceed test case
//	records := []*api.Record{
//		{Value: []byte("first")},
//		{Value: []byte("second")},
//	}
//
//	for _, record := range records {
//		off, err := logs[0].Append(record)
//		require.NoError(t, err)
//
//		require.Eventually(t, func() bool {
//			for j := 0; j < nodeCount; j++ {
//				got, err := logs[j].Read(off)
//				if err != nil {
//					return false
//				}
//
//				record.Offset = off
//				if !reflect.DeepEqual(got, record) {
//					return false
//				}
//			}
//
//			return true
//		}, 500*time.Millisecond, 50*time.Millisecond)
//	}
//
//	servers, err := logs[0].GetServers()
//	require.NoError(t, err)
//	require.Equal(t, nodeCount, len(servers))
//	require.True(t, servers[0].IsLeader)
//	require.False(t, servers[1].IsLeader)
//	// require.False(t, servers[2].IsLeader)
//
//	err = logs[0].Leave("1")
//	require.NoError(t, err)
//
//	time.Sleep(50 * time.Millisecond)
//
//	servers, err = logs[0].GetServers()
//	require.NoError(t, err)
//	require.Equal(t, nodeCount-1, len(servers))
//	require.True(t, servers[0].IsLeader)
//	// require.False(t, servers[1].IsLeader)
//
//	off, err := logs[0].Append(&api.Record{
//		Value: []byte("third"),
//	})
//	require.NoError(t, err)
//
//	time.Sleep(50 * time.Millisecond)
//
//	record, err := logs[1].Read(off)
//	require.IsType(t, api.ErrOffsetOutOfRange{}, err)
//	require.Nil(t, record)
//
//	//record, err = logs[2].Read(off)
//	//require.NoError(t, err)
//	//require.Equal(t, &api.Record{
//	//	Value:  []byte("third"),
//	//	Offset: off,
//	//}, record)
//}
