package discovery

import (
	"fmt"
	"mafia/log/lib"
	"net"

	"github.com/hashicorp/serf/serf"
)

type Config struct {
	NodeName           string
	BindAddr           string
	Tags               map[string]string
	StartJoinAddresses []string
}

type Handler interface {
	Join(name, addr string) error
	Leave(name string) error
}

type Membership struct {
	Config
	handler Handler
	serf    *serf.Serf
	events  chan serf.Event
	logger  lib.Logger
}

func New(handler Handler, config Config, logger lib.Logger) (*Membership, error) {
	c := &Membership{
		Config:  config,
		handler: handler,
		logger:  logger,
	}

	if err := c.setupSerf(); err != nil {
		return nil, lib.Wrap(err, "Unable to setup Serf node")
	}

	return c, nil
}

func (m *Membership) setupSerf() (err error) {
	addr, err := net.ResolveTCPAddr("tcp", m.BindAddr)
	if err != nil {
		return lib.Wrap(err, "Unable to resolve TCP address")
	}

	config := serf.DefaultConfig()
	m.events = make(chan serf.Event)

	config.Init()
	config.MemberlistConfig.BindAddr = addr.IP.String()
	config.MemberlistConfig.BindPort = addr.Port
	config.EventCh = m.events
	config.Tags = m.Tags
	config.NodeName = m.Config.NodeName

	m.serf, err = serf.Create(config)
	if err != nil {
		return lib.Wrap(err, "Unable to create Serf instance")
	}

	go m.eventHandler()

	if m.StartJoinAddresses != nil {
		_, err = m.serf.Join(m.StartJoinAddresses, true)
		if err != nil {
			return lib.Wrap(err, "Unable to join start address")
		}
	}

	return
}

func (m *Membership) eventHandler() {
	for e := range m.events {
		switch e.EventType() {
		case serf.EventMemberJoin:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					continue
				}
				m.handleJoin(member)
			}
		case serf.EventMemberLeave, serf.EventMemberFailed:
			for _, member := range e.(serf.MemberEvent).Members {
				if m.isLocal(member) {
					return
				}
				m.handleLeave(member)
			}
		}
	}
}

func (m *Membership) handleJoin(member serf.Member) {
	if err := m.handler.Join(
		member.Name,
		member.Tags["rpc_addr"],
	); err != nil {
		msg := fmt.Sprintf("logs: failed to join: %s %s", member.Name, member.Tags["rpc_addr"])
		m.logger.Log(lib.Error, msg)
	}
}

func (m *Membership) handleLeave(member serf.Member) {
	if err := m.handler.Leave(
		member.Name,
	); err != nil {
		msg := fmt.Sprintf("logs: failed to leave: %s", member.Name)
		m.logger.Log(lib.Error, msg)
	}
}

func (m *Membership) isLocal(member serf.Member) bool {
	return m.serf.LocalMember().Name == member.Name
}

func (m *Membership) Members() []serf.Member {
	return m.serf.Members()
}

func (m *Membership) Leave() error {
	return m.serf.Leave()
}
