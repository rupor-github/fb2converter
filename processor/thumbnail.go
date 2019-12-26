package processor

import (
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/rupor-github/fb2converter/processor/internal/mobi"
)

// ProduceThumbnail reads input file, extracts or creates thumbnail and stores it.
func ProduceThumbnail(fname, outdir string, w, h int, stretch bool, log *zap.Logger) (created bool, err error) {

	defer func() {
		// Sometimes device will have files we cannot recognize and parse
		if r := recover(); r != nil {
			if err != nil {
				log.Debug("Thumbnail extraction ended with panic", zap.String("file", fname), zap.Error(err), zap.ByteString("stack", debug.Stack()))
			} else {
				log.Debug("Thumbnail extraction ended with panic", zap.String("file", fname), zap.ByteString("stack", debug.Stack()))
			}
			// do not stop on panic - give other files a chance to be processed
			err = nil
		}
	}()

	var r *mobi.Reader
	if r, err = mobi.NewReader(fname, w, h, stretch, log); err != nil {
		return false, err
	}
	return r.SaveResult(outdir)
}
