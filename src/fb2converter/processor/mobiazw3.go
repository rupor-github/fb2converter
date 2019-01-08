package processor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"fb2converter/processor/internal/mobi"
)

// FinalizeMOBI produces final mobi file out of previously saved temporary files.
func (p *Processor) FinalizeMOBI(fname string) error {

	tmp, err := p.generateIntermediateContent(fname)
	if err != nil {
		return errors.Wrap(err, "unable to generate intermediate content")
	}

	if _, err := os.Stat(fname); err == nil {
		if !p.env.Debug {
			return errors.Errorf("output file already exists: %s", fname)
		}
		// NOTE: when debugging - ignore existing file
		p.env.Log.Debug("Overwriting existing file - debug mode", zap.String("file", fname))
		if err = os.Remove(fname); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		if err := os.MkdirAll(filepath.Dir(fname), 0700); err != nil {
			return errors.Wrap(err, "unable to create output directory")
		}
	}

	if p.env.Cfg.Doc.Kindlegen.NoOptimization {
		if err := CopyFile(tmp, fname); err != nil {
			return errors.Wrap(err, "unable to copy resulting MOBI")
		}
	} else {
		splitter, err := mobi.NewSplitter(tmp, p.Book.ID, true, p.env.Cfg.Doc.Kindlegen.RemovePersonal, p.env.Log)
		if err != nil {
			return errors.Wrap(err, "unable to parse intermediate content file")
		}
		if err := splitter.SaveResult(fname); err != nil {
			return errors.Wrap(err, "unable to save resulting MOBI")
		}
		if p.kindlePageMap != APNXNone {
			if err := splitter.SavePageMap(fname, p.kindlePageMap == APNXEInk); err != nil {
				return errors.Wrap(err, "unable to save resulting pagemap")
			}
		}
	}
	return nil
}

// FinalizeAZW3 produces final azw3 file out of previously saved temporary files.
func (p *Processor) FinalizeAZW3(fname string) error {

	tmp, err := p.generateIntermediateContent(fname)
	if err != nil {
		return errors.Wrap(err, "unable to generate intermediate content")
	}

	if _, err := os.Stat(fname); err == nil {
		if !p.env.Debug {
			return errors.Errorf("output file already exists: %s", fname)
		}
		// NOTE: when debugging - ignore existing file
		p.env.Log.Debug("Overwriting existing file - debug mode", zap.String("file", fname))
		if err = os.Remove(fname); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else {
		if err := os.MkdirAll(filepath.Dir(fname), 0700); err != nil {
			return errors.Wrap(err, "unable to create output directory")
		}
	}

	if p.env.Cfg.Doc.Kindlegen.NoOptimization {
		if err := CopyFile(tmp, fname); err != nil {
			return errors.Wrap(err, "unable to copy resulting AZW3")
		}
	} else {
		splitter, err := mobi.NewSplitter(tmp, p.Book.ID, false, p.env.Cfg.Doc.Kindlegen.RemovePersonal, p.env.Log)
		if err != nil {
			return errors.Wrap(err, "unable to parse intermediate content file")
		}
		if err := splitter.SaveResult(fname); err != nil {
			return errors.Wrap(err, "unable to save resulting AZW3")
		}
		if p.kindlePageMap != APNXNone {
			if err := splitter.SavePageMap(fname, p.kindlePageMap == APNXEInk); err != nil {
				return errors.Wrap(err, "unable to save resulting pagemap")
			}
		}
	}
	return nil
}

// generateIntermediateContent produces temporary mobi file, presently by running kindlegen and returns it full path.
func (p *Processor) generateIntermediateContent(fname string) (string, error) {

	workDir := filepath.Join(p.tmpDir, DirContent)
	workFile := strings.TrimSuffix(filepath.Base(fname), filepath.Ext(fname)) + ".mobi"

	args := make([]string, 0, 10)
	args = append(args, filepath.Join(workDir, "content.opf"))
	args = append(args, fmt.Sprintf("-c%d", p.env.Cfg.Doc.Kindlegen.CompressionLevel))
	args = append(args, "-locale", "en")
	if p.env.Cfg.Doc.Kindlegen.Verbose || p.env.Debug {
		args = append(args, "-verbose")
	}
	args = append(args, "-o", workFile)

	cmd := exec.Command(p.kindlegenPath, args...)

	start := time.Now()
	p.env.Log.Debug("kindlegen staring")
	defer func(start time.Time) {
		p.env.Log.Debug("kindlegen done",
			zap.Duration("elapsed", time.Now().Sub(start)),
			zap.String("path", cmd.Path),
			zap.Strings("args", args),
		)
	}(start)

	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "unable to redirect kindlegen stdout")
	}

	if err := cmd.Start(); err != nil {
		return "", errors.Wrap(err, "unable to start kindlegen")
	}

	// read and print kindlegen stdout
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		p.env.Log.Debug(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", errors.Wrap(err, "kindlegen stdout pipe broken")
	}

	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if len(ee.Stderr) > 0 {
				p.env.Log.Error("kindlegen", zap.String("stderr", string(ee.Stderr)), zap.Error(err))
			}
			ws := ee.Sys().(syscall.WaitStatus)
			switch ws.ExitStatus() {
			case 0:
				// success
			case 1:
				// warnings
				p.env.Log.Warn("kindlegen has some warnings, see log for details")
			case 2:
				// error - unable to create mobi
				fallthrough
			default:
				return "", errors.Wrap(err, "kindlegen returned error")
			}
		} else {
			return "", errors.Wrap(err, "kindlegen returned error")
		}
	}
	return filepath.Join(workDir, workFile), nil
}
