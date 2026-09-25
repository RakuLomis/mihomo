package proxysemantics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	C "github.com/metacubex/mihomo/constant"
)

type BuildIdentity struct {
	Product                  string `json:"product"`
	Version                  string `json:"version"`
	GoVersion                string `json:"go_version"`
	VCSRevision              string `json:"vcs_revision,omitempty"`
	VCSModified              *bool  `json:"vcs_modified,omitempty"`
	ExecutableSHA256         string `json:"executable_sha256,omitempty"`
	ExecutableHashStatus     string `json:"executable_hash_status"`
	DependencyManifestSHA256 string `json:"dependency_manifest_sha256,omitempty"`
	SourceTreeSHA256         string `json:"source_tree_sha256,omitempty"`
	SourceTreeHashStatus     string `json:"source_tree_hash_status"`
	DependencyLockSHA256     string `json:"dependency_lock_sha256,omitempty"`
	BuildManifestSHA256      string `json:"build_manifest_sha256,omitempty"`
	InjectedIdentityStatus   string `json:"injected_identity_status"`
	IdentitySource           string `json:"identity_source"`
}

var (
	buildOnce     sync.Once
	buildIdentity BuildIdentity
)

func CurrentBuildIdentity() BuildIdentity {
	buildOnce.Do(loadBuildIdentity)
	return buildIdentity
}

func loadBuildIdentity() {
	identity := BuildIdentity{
		Product:                C.MihomoName,
		Version:                C.Version,
		GoVersion:              runtime.Version(),
		ExecutableHashStatus:   "unavailable",
		SourceTreeHashStatus:   "unavailable",
		InjectedIdentityStatus: "unavailable",
		IdentitySource:         "running_process",
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		settings := make(map[string]string, len(info.Settings))
		for _, setting := range info.Settings {
			settings[setting.Key] = setting.Value
		}
		identity.VCSRevision = settings["vcs.revision"]
		if modified, ok := settings["vcs.modified"]; ok {
			value := modified == "true"
			identity.VCSModified = &value
		}
		identity.DependencyManifestSHA256 = hashBuildInfo(info)
	}
	if executable, err := os.Executable(); err == nil {
		if C.BuildRevision != "" {
			identity.VCSRevision = C.BuildRevision
			identity.InjectedIdentityStatus = "verified_build_flags"
		}
		if C.BuildDirty == "true" || C.BuildDirty == "false" {
			value := C.BuildDirty == "true"
			identity.VCSModified = &value
		}
		if C.SourceTreeDigest != "" {
			identity.SourceTreeSHA256 = C.SourceTreeDigest
			identity.SourceTreeHashStatus = "verified_build_flags"
		}
		identity.DependencyLockSHA256 = C.DependencyLockDigest
		identity.BuildManifestSHA256 = C.BuildManifestDigest
		if digest, err := hashFile(executable); err == nil {
			identity.ExecutableSHA256 = digest
			identity.ExecutableHashStatus = "verified_self"
		} else {
			identity.ExecutableHashStatus = "read_failed"
		}
	}
	buildIdentity = identity
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func hashBuildInfo(info *debug.BuildInfo) string {
	entries := make([]string, 0, len(info.Deps)+len(info.Settings)+2)
	entries = append(entries, "go="+info.GoVersion, "path="+info.Path)
	for _, dependency := range info.Deps {
		entry := fmt.Sprintf("dep=%s@%s#%s", dependency.Path, dependency.Version, dependency.Sum)
		if dependency.Replace != nil {
			entry += fmt.Sprintf("=>%s@%s#%s", dependency.Replace.Path, dependency.Replace.Version, dependency.Replace.Sum)
		}
		entries = append(entries, entry)
	}
	for _, setting := range info.Settings {
		if strings.HasPrefix(setting.Key, "vcs.") || setting.Key == "-buildmode" || setting.Key == "GOARCH" || setting.Key == "GOOS" {
			entries = append(entries, "setting="+setting.Key+"="+setting.Value)
		}
	}
	sort.Strings(entries)
	digest := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return "sha256:" + hex.EncodeToString(digest[:])
}
