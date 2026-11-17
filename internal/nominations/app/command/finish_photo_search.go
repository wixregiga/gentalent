package command

import (
	"context"

	"gentalent/internal/nominations/domain"
	"gentalent/internal/nominations/port"
)

type FinishPhotoSearch struct {
	Nomination domain.NominationNumber
	Found      bool
	Strip      string
	Exhibit    string
}

type FinishPhotoSearchHandler struct {
	Groups port.GroupStore
}

func (h FinishPhotoSearchHandler) Handle(ctx context.Context, cmd FinishPhotoSearch, present port.Present[domain.Nomination]) error {
	return h.Groups.Apply(ctx, "", func(group *domain.Group) ([]port.Reply, error) {
		if !cmd.Found {
			failed, err := group.FailPhotoSearch(cmd.Nomination)
			return present(group, failed, err)
		}
		found, err := group.AttachPhotos(cmd.Nomination, cmd.Strip, cmd.Exhibit)
		return present(group, found, err)
	})
}
