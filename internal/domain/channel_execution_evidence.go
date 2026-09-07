package domain

func ChannelConfirmationEvidence(executionID string, send ChannelSendExecution) (ExecutionEvidence, error) {
	payload, err := SanitizeExecutionJSON(map[string]any{"message_id": send.MessageID, "touchpoint_id": send.MessageID, "execution_id": executionID, "provider": send.Provider, "provider_message_id": send.ProviderMessageID})
	return ExecutionEvidence{ExecutionID: executionID, Key: send.EffectKey + ":confirmed", Kind: "delivery_status", CustomerID: send.CustomerID, DeliveryID: send.MessageID, Status: "confirmed", Payload: payload}, err
}
