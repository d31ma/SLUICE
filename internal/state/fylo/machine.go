package fylo

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
)

// MachineError is a stable Fylo machine-protocol failure. The runtime's
// diagnostic message is deliberately not exposed because it can contain
// filesystem details.
type MachineError struct {
	Code string
}

func (err *MachineError) Error() string {
	return "fylo machine operation failed: " + err.Code
}

var machineRequestSequence atomic.Uint64

func (supervisor *Supervisor) execute(
	ctx context.Context,
	operation string,
	fields map[string]any,
) (json.RawMessage, error) {
	request := make(map[string]any, len(fields)+2)
	request["op"] = operation
	request["requestId"] = fmt.Sprintf(
		"sluice-%s-%d",
		operation,
		machineRequestSequence.Add(1),
	)
	for name, value := range fields {
		request[name] = value
	}
	reply, err := supervisor.request(ctx, request)
	if err != nil {
		return nil, err
	}
	if reply.Op != operation || reply.ProtocolVersion != supervisor.requirements.ProtocolVersion {
		return nil, fmt.Errorf("%w: invalid %s response envelope", ErrCompatibility, operation)
	}
	if !reply.OK {
		code := "EUNKNOWN"
		if reply.Error != nil && reply.Error.Code != "" {
			code = reply.Error.Code
		}
		return nil, &MachineError{Code: code}
	}
	return reply.Result, nil
}
