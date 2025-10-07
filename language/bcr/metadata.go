package bcr

import (
	"fmt"

	bzpb "github.com/stackb/centrl/build/stack/bazel/bzlmod/v1"
	"github.com/stackb/centrl/pkg/protoutil"
)

func readMetadataJson(filename string) (*bzpb.Metadata, error) {
	var md bzpb.Metadata
	if err := protoutil.ReadFile(filename, &md); err != nil {
		return nil, fmt.Errorf("reading metadata json: %v", err)
	}
	return &md, nil
}
