package domain

type GenerationalTalent struct{}

func (GenerationalTalent) SatisfiedBy(status CertificationStatus) bool {
	return status == Certified
}
