# v44 channel providers and receipt ledger implementation plan

1. Add failing domain tests for SMS/push integration validation, encryption, redaction and update preservation.
2. Implement Twilio and FCM provider settings plus workspace integration wiring.
3. Add failing adapter tests using local HTTP servers, then implement bounded Twilio/FCM send clients and error classification.
4. Add v44 receipt schema/migration, domain repository contract and PostgreSQL implementation with duplicate/hash-conflict behavior.
5. Add authenticated batch receipt ingestion and signed Twilio callback handlers with service tests.
6. Update console API types, changelog and README.
7. Run focused/full verification, rebuild Compose, exercise migration and local HTTP receipt ingestion, then commit on `dev`.
