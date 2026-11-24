package deployment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing/iotest"

	astrov1alpha1_mocks "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1/mocks"
)

func (s *Suite) TestBundleLimits() {
	s.Run("a copy names every file that breaks a limit, before making any request", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"ok.py": "ok", "my report.sql": "x", "__DEL__x.py": "", "lib/__MV__y.py": ""})
		s.NoError(os.WriteFile(filepath.Join(fromDir, "big.bin"), make([]byte, maxBundleFileBytes+1), 0o600))

		err := CopyBundleFiles(context.Background(), fromDir, "my-files:dags", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, bundleOut(&bytes.Buffer{}), mockV1Client, new(astrov1alpha1_mocks.ClientWithResponsesInterface), s.streamClient())
		var limitErr *bundleLimitError
		s.ErrorAs(err, &limitErr)
		s.Len(limitErr.faults, 4)
		s.ErrorContains(err, "nothing was written, because these break the bundle limits:\n")
		s.ErrorContains(err, filepath.Join(fromDir, "big.bin")+" is 1048577 bytes, past the 1048576 a bundle file may hold")
		s.ErrorContains(err, filepath.Join(fromDir, "my report.sql")+": its bundle path dags/my report.sql may hold only ASCII letters, digits, '/', '.', '_' and '-'")
		s.ErrorContains(err, `its bundle path dags/__DEL__x.py has a segment beginning "__DEL__", which a directory write reserves`)
		s.ErrorContains(err, `its bundle path dags/lib/__MV__y.py has a segment beginning "__MV__", which a directory write reserves`)
	})

	s.Run("counts the bundle directory in a path's length", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"ok.py": "ok"})
		dir := strings.Repeat("d", 190)

		err := CopyBundleFiles(context.Background(), fromDir, "my-files:"+dir, &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, bundleOut(&bytes.Buffer{}), mockV1Client, new(astrov1alpha1_mocks.ClientWithResponsesInterface), s.streamClient())
		s.ErrorContains(err, "its bundle path "+dir+"/ok.py is 196 bytes, past the 192 allowed")
	})

	s.Run("refuses more files than a bundle may hold", func() {
		err := checkBundleUpload("./dags", "", make([]string, maxBundleFiles+1))
		s.EqualError(err, "nothing was written, because these break the bundle limits:\n  ./dags holds 10001 files, past the 10000 a bundle may hold")
	})

	s.Run("refuses more bytes than a bundle may hold", func() {
		fromDir := s.T().TempDir()
		files := make([]string, 101)
		for i := range files {
			files[i] = fmt.Sprintf("part%d.bin", i)
			filePath := filepath.Join(fromDir, files[i])
			s.NoError(os.WriteFile(filePath, nil, 0o600))
			s.NoError(os.Truncate(filePath, maxBundleFileBytes))
		}

		err := checkBundleUpload(fromDir, "", files)
		s.EqualError(err, "nothing was written, because these break the bundle limits:\n  "+fromDir+" holds 105906176 bytes, past the 104857600 a bundle may hold")
		s.NoError(checkBundleUpload(fromDir, "", files[:100]))
	})

	s.Run("shows the first faults and counts the rest", func() {
		faults := make([]string, maxShownLimitFaults+5)
		for i := range faults {
			faults[i] = fmt.Sprintf("fault %d", i)
		}
		err := &bundleLimitError{faults: faults}
		s.Contains(err.Error(), "\n  fault 19\n  and 5 more")
		s.NotContains(err.Error(), "fault 20")
	})

	s.Run("cp -r checks the directory before making any request", func() {
		fromDir := s.T().TempDir()
		writeLocalFiles(s, fromDir, map[string]string{"ok.py": "ok", "bad name.py": "x"})

		err := CopyBundleFiles(context.Background(), fromDir, "my-files:dags/", &BundleCopyOptions{Recursive: true}, ws, testBundleDeploymentID, bundleOut(&bytes.Buffer{}), mockV1Client, new(astrov1alpha1_mocks.ClientWithResponsesInterface), s.streamClient())
		s.ErrorContains(err, "its bundle path dags/bad name.py may hold only")
	})

	s.Run("reads no more of an upload than one byte past what a bundle file may hold", func() {
		content, err := readBundleUpload(bytes.NewReader(make([]byte, maxBundleFileBytes)), "stdin")
		s.NoError(err)
		s.Equal(int64(maxBundleFileBytes), content.Size())

		_, err = readBundleUpload(io.MultiReader(bytes.NewReader(make([]byte, maxBundleFileBytes+1)), iotest.ErrReader(errors.New("read too far"))), "stdin")
		s.EqualError(err, "nothing was written, because these break the bundle limits:\n  stdin holds more than the 1048576 bytes a bundle file may hold")
	})

	s.Run("cp checks a file's size and its bundle path", func() {
		big := filepath.Join(s.T().TempDir(), "big.bin")
		s.NoError(os.WriteFile(big, make([]byte, maxBundleFileBytes+1), 0o600))

		err := CopyBundleFiles(context.Background(), big, "my-files:dags/", &BundleCopyOptions{}, ws, testBundleDeploymentID, bundleOut(&bytes.Buffer{}), mockV1Client, new(astrov1alpha1_mocks.ClientWithResponsesInterface), s.streamClient())
		s.ErrorContains(err, big+" is 1048577 bytes, past the 1048576 a bundle file may hold")

		err = CopyBundleFiles(context.Background(), "-", "my-files:bad name.py", &BundleCopyOptions{}, ws, testBundleDeploymentID, bundleOut(&bytes.Buffer{}), mockV1Client, new(astrov1alpha1_mocks.ClientWithResponsesInterface), s.streamClient())
		s.ErrorContains(err, "stdin: its bundle path bad name.py may hold only")
	})
}
