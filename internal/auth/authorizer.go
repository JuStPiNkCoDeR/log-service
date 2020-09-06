package auth

import (
	"mafia/log/lib"

	"fmt"

	"github.com/casbin/casbin"
	"google.golang.org/grpc/codes"
)

type Authorizer struct {
	enforcer *casbin.Enforcer
}

func New(model, policy string) *Authorizer {
	enforcer := casbin.NewEnforcer(model, policy)

	return &Authorizer{
		enforcer: enforcer,
	}
}

func (a *Authorizer) Authorize(subject, object, action string) error {
	if !a.enforcer.Enforce(subject, object, action) {
		msg := fmt.Sprintf("%s not permitted to %s to %s", subject, action, object)
		st := lib.CreateStatusErr(codes.PermissionDenied, "Authorize denied", "en-US", msg)

		return st.Err()
	}

	return nil
}
