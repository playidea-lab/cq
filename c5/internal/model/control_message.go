package model

// ControlMessageRequest is the payload for POST /v1/edges/{id}/control.
type ControlMessageRequest struct {
	Action string            `json:"action"`
	Params map[string]string `json:"params,omitempty"`
}

// EdgeControlMessage is a control message stored in the queue.
type EdgeControlMessage struct {
	MessageID string            `json:"message_id"`
	Action    string            `json:"action"`
	Params    map[string]string `json:"params,omitempty"`
}

// ControlEnqueueResponse is returned from POST /v1/edges/{id}/control.
type ControlEnqueueResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}
