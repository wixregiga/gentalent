package domain

type Ref struct {
	Number NominationNumber
	Person PersonID
}

func (r Ref) Unnamed() bool {
	return r == Ref{}
}
