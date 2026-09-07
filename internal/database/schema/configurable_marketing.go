package schema

// ConfigurableMarketingTableDefinitions keeps fresh workspace setup and the
// upgrade path in the same dependency order. Existing customer, event ledger,
// campaign and journey tables must be installed first.
func ConfigurableMarketingTableDefinitions() []string {
	var queries []string
	queries = append(queries, BusinessConfigTableDefinitions()...)
	queries = append(queries, TrackedEventTableDefinitions()...)
	queries = append(queries, SubjectStateTableDefinitions()...)
	queries = append(queries, SubjectAttributeHistoryTableDefinitions()...)
	queries = append(queries, FunnelTableDefinitions()...)
	queries = append(queries, ExecutionSnapshotTableDefinitions()...)
	queries = append(queries, ExecutionRuntimeSnapshotDefinitions()...)
	queries = append(queries, ConfigurableRuntimeTableDefinitions()...)
	queries = append(queries, ExecutionEventMetadataDefinitions()...)
	queries = append(queries, CustomerJourneyExecutionDefinitions()...)
	queries = append(queries, CustomerChannelDefinitions()...)
	return queries
}
