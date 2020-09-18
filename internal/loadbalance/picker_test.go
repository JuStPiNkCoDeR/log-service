package loadbalance

import (
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/resolver"
	"testing"
)

func TestPickerProducesToLeader(t *testing.T) {
	picker, subConnes := setupTest()
	info := balancer.PickInfo{
		FullMethodName: "/log.vX.Log/Produce",
	}
	for i := 0; i < 5; i++ {
		gotPick, err := picker.Pick(info)
		require.NoError(t, err)
		require.Equal(t, subConnes[0], gotPick.SubConn)
	}
}

func TestPickerConsumesFromFollowers(t *testing.T) {
	picker, subConnes := setupTest()
	info := balancer.PickInfo{
		FullMethodName: "/log.vX.Log/Consume",
	}
	for i := 0; i < 5; i++ {
		pick, err := picker.Pick(info)
		require.NoError(t, err)
		require.Equal(t, subConnes[i%2+1], pick.SubConn)
	}
}

// subConn implements balancer.SubConn.
type subConn struct {
	addresses []resolver.Address
}

func (s *subConn) UpdateAddresses(addresses []resolver.Address) {
	s.addresses = addresses
}
func (s *subConn) Connect() {}

func setupTest() (*Picker, []*subConn) {
	var subConnes []*subConn
	buildInfo := base.PickerBuildInfo{
		ReadySCs: make(map[balancer.SubConn]base.SubConnInfo),
	}

	for i := 0; i < 3; i++ {
		sc := &subConn{}
		address := resolver.Address{
			Attributes: attributes.New("is_leader", i == 0),
		}
		// 0th sub conn is the leader
		sc.UpdateAddresses([]resolver.Address{address})
		buildInfo.ReadySCs[sc] = base.SubConnInfo{Address: address}
		subConnes = append(subConnes, sc)
	}

	picker := &Picker{}
	picker.Build(buildInfo)
	return picker, subConnes
}
