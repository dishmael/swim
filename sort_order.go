package swim

// SortOrder ...
type SortOrder int

const (
	// ASC declares ascending sort order
	ASC = iota
	// DESC declares descending sort order
	DESC
)

func (s SortOrder) String() string {
	return [...]string{"ASC", "DESC"}[s]
}
