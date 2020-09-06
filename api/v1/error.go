package log_v1

import (
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"mafia/log/lib"
)

type ErrOffsetOutOfRange struct {
	Offset uint64
}

func (e ErrOffsetOutOfRange) GRPCStatus() *status.Status {
	return lib.CreateStatusErr(
		codes.NotFound,
		fmt.Sprintf("Offset out of range: %d", e.Offset),
		"en-US",
		fmt.Sprintf("The requested offset is outside the log's range: %d", e.Offset),
	)
}

func (e ErrOffsetOutOfRange) Error() string {
	return e.GRPCStatus().Err().Error()
}
