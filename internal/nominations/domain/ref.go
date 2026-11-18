package domain

type Ref struct {
	Number NominationNumber
	Person PersonID
	Close  []PersonID
}

func (r Ref) Unnamed() bool {
	return r.Number == 0 && r.Person == "" && len(r.Close) == 0
}
