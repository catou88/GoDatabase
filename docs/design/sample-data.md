# Sample data and operation outcomes

An initial dataset contains keys key-000000 through key-(size-1), zero padded
to six digits. Values use the first six bytes of SHA-256 of seed:index,
encoded as hexadecimal after record-. Seed and size reproduce identical data.
The default nonempty lookup should target key-000000.

Responses echo the submitted request and contain at most 64 initial sample
records. dataset_truncated identifies a partial preview. Operations return
stored, found, not_found, deleted, matched, or empty status. Empty string values
and false found flags are explicit. Range count is the complete match count;
entries contains at most 64 records, with truncated marking omitted records.
The initial dataset preview is not a post-mutation snapshot.

The frontend workload editor and record selection are integrated in #192.
