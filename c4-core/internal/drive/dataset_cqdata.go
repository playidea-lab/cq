package drive

import (
	"github.com/changmin/c4-core/internal/cqdata"
)

// ApplyCQData loads .cqdata from dir, sets the dataset entry for name, and saves.
// The version stored is the full versionHash.
func ApplyCQData(dir, name, versionHash string) error {
	cd, err := cqdata.Load(dir)
	if err != nil {
		return err
	}
	cd.SetDataset(name, name, versionHash)
	return cd.Save(dir)
}
