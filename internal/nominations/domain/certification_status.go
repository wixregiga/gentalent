package domain

type CertificationStatus string

const (
	Nominated CertificationStatus = "nominated"
	Certified CertificationStatus = "certified"
	Rejected  CertificationStatus = "rejected"
)
