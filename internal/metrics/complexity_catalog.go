package metrics

type catalog map[Structure]map[Operation]Metadata

// DefaultCatalog returns the complexity metadata for every supported
// structure and operation.
func DefaultCatalog() Catalog {
	return catalog{
		Map: entries(Map, map[Operation]Complexity{
			Set:    {Best: "O(1)", Average: "O(1)", Worst: "O(n)", Memory: "O(n)", Assumptions: "Hashing is expected constant time; n is the number of entries."},
			Get:    {Best: "O(1)", Average: "O(1)", Worst: "O(n)", Memory: "O(n)", Assumptions: "Hashing is expected constant time; n is the number of entries."},
			Delete: {Best: "O(1)", Average: "O(1)", Worst: "O(n)", Memory: "O(n)", Assumptions: "Hashing is expected constant time; n is the number of entries."},
			Range:  {Best: "O(n)", Average: "O(n)", Worst: "O(n)", Memory: "O(n + k)", Assumptions: "The reference map scans and sorts keys; k is the number of returned entries."},
		}),
		SortedSlice: entries(SortedSlice, map[Operation]Complexity{
			Set:    {Best: "O(1)", Average: "O(n)", Worst: "O(n)", Memory: "O(n)", Assumptions: "Lookup is binary search, but inserting into the sorted slice shifts entries."},
			Get:    {Best: "O(1)", Average: "O(log n)", Worst: "O(log n)", Memory: "O(n)"},
			Delete: {Best: "O(1)", Average: "O(n)", Worst: "O(n)", Memory: "O(n)", Assumptions: "Removing an entry shifts later entries."},
			Range:  {Best: "O(log n + k)", Average: "O(log n + k)", Worst: "O(n)", Memory: "O(n + k)", Assumptions: "k is the number of returned entries; a full-range result is O(n)."},
		}),
		BTreeMemory: entries(BTreeMemory, map[Operation]Complexity{
			Set:    {Best: "O(log n)", Average: "O(log n)", Worst: "O(log n)", Memory: "O(n)", Assumptions: "The tree remains balanced and node work is bounded."},
			Get:    {Best: "O(1)", Average: "O(log n)", Worst: "O(log n)", Memory: "O(n)"},
			Delete: {Best: "O(log n)", Average: "O(log n)", Worst: "O(log n)", Memory: "O(n)", Assumptions: "Rebalancing work is bounded per tree level."},
			Range:  {Best: "O(log n + k)", Average: "O(log n + k)", Worst: "O(log n + k)", Memory: "O(n + k)", Assumptions: "k is the number of returned entries and leaves are ordered."},
		}),
		BTreeDurable: entries(BTreeDurable, map[Operation]Complexity{
			Set:    {Best: "O(log_B n)", Average: "O(log_B n)", Worst: "O(log_B n)", Memory: "O(n)", Assumptions: "B is the page fanout; excludes variable fsync latency."},
			Get:    {Best: "O(1)", Average: "O(log_B n)", Worst: "O(log_B n)", Memory: "O(n)", Assumptions: "A cached root can make the first step O(1); disk I/O latency is measured separately."},
			Delete: {Best: "O(log_B n)", Average: "O(log_B n)", Worst: "O(log_B n)", Memory: "O(n)", Assumptions: "Includes copy-on-write path replacement and bounded rebalancing."},
			Range:  {Best: "O(log_B n + k)", Average: "O(log_B n + k)", Worst: "O(log_B n + k)", Memory: "O(n + k)", Assumptions: "k is the number of returned entries; page reads and synchronization are measured separately."},
		}),
	}
}

func entries(structure Structure, values map[Operation]Complexity) map[Operation]Metadata {
	result := make(map[Operation]Metadata, len(values))
	for operation, complexity := range values {
		result[operation] = Metadata{Structure: structure, Operation: operation, Complexity: complexity}
	}
	return result
}

func (c catalog) Lookup(structure Structure, operation Operation) (Metadata, bool) {
	operations, ok := c[structure]
	if !ok {
		return Metadata{}, false
	}
	metadata, ok := operations[operation]
	return metadata, ok
}
