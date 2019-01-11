package processor

import (
	"go.uber.org/zap"

	"fb2converter/processor/internal/mobi"
)

// ProduceThumbnail reads input file, extracts or creates thumbnail and stores it.
func ProduceThumbnail(fname, outdir string, w, h int, stretch bool, log *zap.Logger) (bool, error) {

	r, err := mobi.NewReader(fname, w, h, stretch, log)
	if err != nil {
		return false, err
	}
	return r.SaveResult(outdir)
}
