# Channel message send implementation plan

1. Extend encrypted contact endpoints and workspace migration for Twilio phone destinations.
2. Add a provider-neutral send request, durable execution ledger and payload-conflict contract.
3. Reuse saved template resolution, multilingual Liquid rendering and contact data.
4. Atomically confirm provider acceptance with encrypted message history.
5. Expose the authenticated HTTP API and TypeScript client contract.
6. Add SMS/push automation nodes that reuse realtime journey effect keys.
7. Verify domain, repository, service, HTTP, migration, console build and Compose runtime behavior.
