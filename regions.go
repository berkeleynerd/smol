package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	regionAuthoredContent     = "authored-content"
	regionGeneratedNavigation = "generated-navigation"

	authoredContentOpen      = `<!--smol:authored-content-->`
	authoredContentClose     = `<!--/smol:authored-content-->`
	generatedNavigationOpen  = `<!--smol:generated-navigation-->`
	generatedNavigationClose = `<!--/smol:generated-navigation-->`
)

type regionSpec struct {
	name  string
	open  string
	close string
}

var regionSpecs = []regionSpec{
	{
		name:  regionAuthoredContent,
		open:  authoredContentOpen,
		close: authoredContentClose,
	},
	{
		name:  regionGeneratedNavigation,
		open:  generatedNavigationOpen,
		close: generatedNavigationClose,
	},
}

func regionHashes(html string) (map[string]string, error) {
	hashes := map[string]string{}
	for _, spec := range regionSpecs {
		hash, ok, err := regionHash(html, spec)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		hashes[spec.name] = hash
	}
	return hashes, nil
}

func validateNoReservedRegionMarkers(page Page, html string) error {
	for _, marker := range reservedRegionMarkers() {
		if strings.Contains(html, marker) {
			return fmt.Errorf("%s %q rendered body contains reserved region marker %q", page.Kind, page.Slug, marker)
		}
	}
	return nil
}

func regionHash(html string, spec regionSpec) (string, bool, error) {
	count := strings.Count(html, spec.open)
	if count == 0 {
		return "", false, nil
	}
	if count > 1 {
		return "", false, fmt.Errorf("region %q open marker appears %d times; region markers are reserved and must occur exactly once", spec.name, count)
	}
	innerStart := strings.Index(html, spec.open) + len(spec.open)
	closeAt := strings.Index(html[innerStart:], spec.close)
	if closeAt < 0 {
		return "", false, fmt.Errorf("region %q missing exact close marker %q", spec.name, spec.close)
	}
	sum := sha256.Sum256([]byte(html[innerStart : innerStart+closeAt]))
	return hex.EncodeToString(sum[:]), true, nil
}

func manifestRegionsFromHashes(hashes map[string]string) []ManifestRegion {
	if len(hashes) == 0 {
		return nil
	}
	regions := []ManifestRegion{}
	for _, spec := range regionSpecs {
		if hash, ok := hashes[spec.name]; ok {
			regions = append(regions, ManifestRegion{Name: spec.name, SHA256: hash})
		}
	}
	return regions
}

func manifestRegionHashes(regions []ManifestRegion) map[string]string {
	hashes := map[string]string{}
	for _, region := range regions {
		if isKnownRegionName(region.Name) {
			hashes[region.Name] = region.SHA256
		}
	}
	return hashes
}

func compareRegionHashes(expected, actual map[string]string) error {
	for _, name := range sortedRegionNames(expected) {
		expectedHash := expected[name]
		actualHash, ok := actual[name]
		if !ok {
			return fmt.Errorf("region %q declared in manifest but not found in HTML", name)
		}
		if actualHash != expectedHash {
			return fmt.Errorf("region %q hash mismatch; region hashes are byte-exact and line-ending-sensitive", name)
		}
	}
	for _, name := range sortedRegionNames(actual) {
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("region %q found in HTML but missing from manifest", name)
		}
	}
	return nil
}

func compareDeclaredRegionHashes(regions []ManifestRegion, html string) error {
	expected := manifestRegionHashes(regions)
	for _, spec := range regionSpecs {
		expectedHash, ok := expected[spec.name]
		if !ok {
			continue
		}
		actualHash, found, err := regionHash(html, spec)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("region %q declared in manifest but not found in HTML", spec.name)
		}
		if actualHash != expectedHash {
			return fmt.Errorf("region %q hash mismatch; region hashes are byte-exact and line-ending-sensitive", spec.name)
		}
	}
	return nil
}

func isKnownRegionName(name string) bool {
	for _, spec := range regionSpecs {
		if spec.name == name {
			return true
		}
	}
	return false
}

func sortedRegionNames(values map[string]string) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func reservedRegionMarkers() []string {
	return []string{authoredContentOpen, authoredContentClose, generatedNavigationOpen, generatedNavigationClose}
}
