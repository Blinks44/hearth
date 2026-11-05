package pkgmgr

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// chocoLocal reads the packages in Chocolatey's lib folder, as choco list
// does, instead of running choco.exe: it starts in seconds. The chocolatey
// package there is Chocolatey's own version.
func chocoLocal(m *inventory.PackageManager) {
	root := filepath.Dir(m.Path)
	if strings.EqualFold(filepath.Base(root), "bin") {
		root = filepath.Dir(root)
	}
	dirs, err := os.ReadDir(filepath.Join(root, "lib"))
	if err != nil {
		m.Warnings = append(m.Warnings, fmt.Sprintf("read packages: %v", err))
		return
	}
	for _, dir := range dirs {
		path := filepath.Join(root, "lib", dir.Name())
		if !dir.IsDir() {
			continue
		}
		// An install still running or one that failed midway.
		if _, err := os.Stat(filepath.Join(path, ".chocolateyPending")); err == nil {
			continue
		}
		id, version, err := nuspec(path, dir.Name())
		if err != nil {
			m.Warnings = append(m.Warnings, dir.Name()+": "+err.Error())
			continue
		}
		if strings.EqualFold(id, "chocolatey") {
			m.Version = version
		}
		m.Packages = append(m.Packages, inventory.SystemPackage{Name: id, Version: version, Path: path, Kind: "package"})
	}
}

// nuspec reads the id and version of the package in dir: from its .nuspec,
// or from inside its .nupkg, which is all Chocolatey's install script leaves
// for the chocolatey package itself.
func nuspec(dir, name string) (id, version string, err error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".nuspec"))
	if errors.Is(err, os.ErrNotExist) {
		data, err = nupkgSpec(filepath.Join(dir, name+".nupkg"))
	}
	if err != nil {
		return "", "", fmt.Errorf("read nuspec: %w", err)
	}
	var spec struct {
		Metadata struct {
			ID      string `xml:"id"`
			Version string `xml:"version"`
		} `xml:"metadata"`
	}
	if err := xml.Unmarshal(data, &spec); err != nil {
		return "", "", fmt.Errorf("read nuspec: %w", err)
	}
	if spec.Metadata.ID == "" || spec.Metadata.Version == "" {
		return "", "", errors.New("nuspec has no id or version")
	}
	return spec.Metadata.ID, nugetVersion(spec.Metadata.Version), nil
}

// nupkgSpec is the .nuspec at the root of the package archive path.
func nupkgSpec(path string) ([]byte, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }() // read only
	for _, f := range r.File {
		if !strings.Contains(f.Name, "/") && strings.EqualFold(filepath.Ext(f.Name), ".nuspec") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(io.LimitReader(rc, 1<<20))
		}
	}
	return nil, errors.New("no nuspec in " + filepath.Base(path))
}

// nugetVersion is version as Chocolatey 2 prints it, NuGet's normalized
// form: no leading zeros, at least three parts, a fourth only when not 0,
// no build metadata. Anything else is kept as written.
func nugetVersion(version string) string {
	version, _, _ = strings.Cut(strings.TrimSpace(version), "+")
	release, pre, hasPre := strings.Cut(version, "-")
	parts := strings.Split(release, ".")
	if len(parts) > 4 {
		return version
	}
	nums := make([]string, 0, 4)
	for _, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return version
		}
		nums = append(nums, strconv.FormatUint(n, 10))
	}
	for len(nums) < 3 {
		nums = append(nums, "0")
	}
	if len(nums) == 4 && nums[3] == "0" {
		nums = nums[:3]
	}
	out := strings.Join(nums, ".")
	if hasPre {
		out += "-" + pre
	}
	return out
}
