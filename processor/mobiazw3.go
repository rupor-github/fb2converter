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

	"github.com/google/uuid"
	"go.uber.org/zap"

	"fb2converter/processor/internal/mobi"
)

// FinalizeMOBI produces final mobi file out of previously saved temporary files.
func (p *Processor) FinalizeMOBI(fname string) error {

	tmp, err := p.generateIntermediateContent(fname)
	if err != nil {
		return fmt.Errorf("unable to generate intermediate content: %w", err)
	}

	if _, err := os.Stat(fname); err == nil {
		if !p.env.Debug && !p.overwrite {
			return fmt.Errorf("output file already exists: %s", fname)
		}
		p.env.Log.Warn("Overwriting existing file", zap.String("file", fname))
		if err = os.Remove(fname); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err := os.MkdirAll(filepath.Dir(fname), 0700); err != nil {
		return fmt.Errorf("unable to create output directory: %w", err)
	}

	if p.env.Cfg.Doc.Kindlegen.NoOptimization {
		if err := CopyFile(tmp, fname); err != nil {
			return fmt.Errorf("unable to copy resulting MOBI: %w", err)
		}
	} else {
		var u uuid.UUID
		var a string
		if p.Book == nil {
			u, err = uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("unable to generate UUID: %w", err)
			}
		} else {
			u = p.Book.ID
			a = p.Book.ASIN
		}
		splitter, err := mobi.NewSplitter(tmp, u, a, true, p.env.Cfg.Doc.Kindlegen.RemovePersonal, false, p.env.Log)
		if err != nil {
			return fmt.Errorf("unable to parse intermediate content file: %w", err)
		}
		if err := splitter.SaveResult(fname); err != nil {
			return fmt.Errorf("unable to save resulting MOBI: %w", err)
		}
		if p.kindlePageMap != APNXNone {
			if err := splitter.SavePageMap(fname, p.kindlePageMap == APNXEInk); err != nil {
				return fmt.Errorf("unable to save resulting pagemap: %w", err)
			}
		}
	}
	return nil
}

// FinalizeAZW3 produces final azw3 file out of previously saved temporary files.
func (p *Processor) FinalizeAZW3(fname string) error {

	tmp, err := p.generateIntermediateContent(fname)
	if err != nil {
		return fmt.Errorf("unable to generate intermediate content: %w", err)
	}

	if _, err := os.Stat(fname); err == nil {
		if !p.env.Debug && !p.overwrite {
			return fmt.Errorf("output file already exists: %s", fname)
		}
		p.env.Log.Warn("Overwriting existing file", zap.String("file", fname))
		if err = os.Remove(fname); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err := os.MkdirAll(filepath.Dir(fname), 0700); err != nil {
		return fmt.Errorf("unable to create output directory: %w", err)
	}

	if p.env.Cfg.Doc.Kindlegen.NoOptimization {
		if err := CopyFile(tmp, fname); err != nil {
			return fmt.Errorf("unable to copy resulting AZW3: %w", err)
		}
	} else {
		var u uuid.UUID
		var a string
		if p.Book == nil {
			u, err = uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("unable to generate UUID: %w", err)
			}
		} else {
			u = p.Book.ID
			a = p.Book.ASIN
		}
		splitter, err := mobi.NewSplitter(tmp, u, a, false, p.env.Cfg.Doc.Kindlegen.RemovePersonal, p.env.Cfg.Doc.Kindlegen.ForceASIN, p.env.Log)
		if err != nil {
			return fmt.Errorf("unable to parse intermediate content file: %w", err)
		}
		if err := splitter.SaveResult(fname); err != nil {
			return fmt.Errorf("unable to save resulting AZW3: %w", err)
		}
		if p.kindlePageMap != APNXNone {
			if err := splitter.SavePageMap(fname, p.kindlePageMap == APNXEInk); err != nil {
				return fmt.Errorf("unable to save resulting pagemap: %w", err)
			}
		}
	}
	return nil
}

// generateIntermediateContent produces temporary mobi file, presently by running kindlegen and returns its full path.
func (p *Processor) generateIntermediateContent(fname string) (string, error) {

	workDir := filepath.Join(p.tmpDir, DirContent)
	if p.kind == InEpub {
		workDir = p.tmpDir
	}
	workFile := strings.TrimSuffix(filepath.Base(fname), filepath.Ext(fname)) + ".mobi"

	args := make([]string, 0, 10)
	if p.kind == InEpub {
		args = append(args, filepath.Join(p.tmpDir, filepath.Base(p.src)))
	} else {
		args = append(args, filepath.Join(workDir, "content.opf"))
	}
	args = append(args, fmt.Sprintf("-c%d", p.env.Cfg.Doc.Kindlegen.CompressionLevel))
	args = append(args, "-locale", "en")
	if p.env.Cfg.Doc.Kindlegen.Verbose || p.env.Debug {
		args = append(args, "-verbose")
	}
	args = append(args, "-o", workFile)

	cmd := exec.Command(p.kindlegenPath, args...)

	p.env.Log.Debug("kindlegen staring")
	defer func(start time.Time) {
		p.env.Log.Debug("kindlegen done",
			zap.Duration("elapsed", time.Since(start)),
			zap.String("path", cmd.Path),
			zap.Strings("args", args),
		)
	}(time.Now())

	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("unable to redirect kindlegen stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("unable to start kindlegen: %w", err)
	}

	// read and print kindlegen stdout
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		p.env.Log.Debug(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("kindlegen stdout pipe broken: %w", err)
	}

	result := filepath.Join(workDir, workFile)
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if len(ee.Stderr) > 0 {
				p.env.Log.Error("kindlegen", zap.String("stderr", string(ee.Stderr)), zap.Error(err))
			}
			ws := ee.Sys().(syscall.WaitStatus)
			switch ws.ExitStatus() {
			case 1:
				// warnings
				p.env.Log.Warn("kindlegen has some warnings, see log for details")
				fallthrough
			case 0:
				// success
				if _, err := os.Stat(result); err != nil {
					// kindlegen lied
					return "", fmt.Errorf("kindlegen did not return an error, but there is no content %s: %w", result, err)
				}
			case 2:
				// error - unable to create mobi
				fallthrough
			default:
				return "", fmt.Errorf("kindlegen returned error: %w", err)
			}
		} else {
			return "", fmt.Errorf("kindlegen returned error: %w", err)
		}
	}
	return result, nil
}
