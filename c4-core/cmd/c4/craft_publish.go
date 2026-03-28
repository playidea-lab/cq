package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/changmin/c4-core/internal/craft"
	"github.com/spf13/cobra"
)

var craftPublishBump string

var craftPublishCmd = &cobra.Command{
	Use:   "publish <path>",
	Short: "스킬을 레지스트리에 게시 (관리자 전용)",
	Long: `로컬 스킬/에이전트/룰을 CQ 레지스트리에 게시합니다.

Phase 1: 관리자만 게시 가능. Supabase RLS로 강제.

사용 예시:
  cq publish ./my-skill/              스킬 디렉토리 게시 (SKILL.md 필요)
  cq publish ./my-rule.md             단일 파일 게시
  cq publish ./my-skill/ --bump minor 마이너 버전 bump`,
	Args: cobra.ExactArgs(1),
	RunE: runCraftPublish,
}

func init() {
	craftPublishCmd.Flags().StringVar(&craftPublishBump, "bump", "patch", "버전 bump 타입 (patch|minor|major)")
	rootCmd.AddCommand(craftPublishCmd)
}

func runCraftPublish(cmd *cobra.Command, args []string) error {
	path := args[0]

	client, err := newRegistryClientAuthenticated()
	if err != nil {
		return fmt.Errorf("인증 실패: %w", err)
	}

	// Read local skill.
	skill, version, err := readLocalSkill(path)
	if err != nil {
		return fmt.Errorf("스킬 읽기 실패: %w", err)
	}

	// Check if already exists and determine version.
	existing, err := client.Search(skill.Name)
	if err != nil {
		return fmt.Errorf("레지스트리 조회 실패: %w", err)
	}

	if existing != nil {
		newVer, bumpErr := bumpVersion(existing.LatestVersion, craftPublishBump)
		if bumpErr != nil {
			return fmt.Errorf("버전 bump 실패: %w", bumpErr)
		}
		version.Version = newVer
		fmt.Printf("기존 스킬 발견: %s@%s → %s\n", skill.Name, existing.LatestVersion, newVer)
	} else {
		version.Version = "1.0.0"
		fmt.Printf("새 스킬: %s@1.0.0\n", skill.Name)
	}

	skill.LatestVersion = version.Version

	if err := client.Publish(skill, version); err != nil {
		if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "new row violates") {
			return fmt.Errorf("게시 실패: 관리자 권한 필요 (Phase 1)")
		}
		return fmt.Errorf("게시 실패: %w", err)
	}

	fmt.Printf("✓ %s@%s 게시 완료 (registry)\n", skill.Name, version.Version)
	return nil
}

// readLocalSkill reads a skill from a local path and returns registry structs.
func readLocalSkill(path string) (craft.RegistrySkill, craft.RegistryVersion, error) {
	info, err := os.Stat(path)
	if err != nil {
		return craft.RegistrySkill{}, craft.RegistryVersion{}, err
	}

	var skill craft.RegistrySkill
	var version craft.RegistryVersion

	if info.IsDir() {
		// Directory — expect SKILL.md.
		skillPath := filepath.Join(path, "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			return skill, version, fmt.Errorf("SKILL.md not found in %s", path)
		}

		skill.Name = filepath.Base(path)
		skill.Type = "skill"
		skill.Description = craft.ParseDescription(content)
		version.Content = string(content)

		// Collect extra files recursively (references/, examples/, scripts/, etc.).
		extraFiles := map[string]string{}
		_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(path, p)
			if rel == "SKILL.md" {
				return nil
			}
			data, readErr := os.ReadFile(p)
			if readErr != nil {
				return nil
			}
			extraFiles[rel] = string(data)
			return nil
		})
		if len(extraFiles) > 0 {
			j, _ := json.Marshal(extraFiles)
			version.ExtraFiles = j
		}
	} else {
		// Single file.
		content, err := os.ReadFile(path)
		if err != nil {
			return skill, version, err
		}

		name := strings.TrimSuffix(filepath.Base(path), ".md")
		skill.Name = name
		skill.Description = craft.ParseDescription(content)
		version.Content = string(content)

		// Determine type from frontmatter.
		if skill.Description != "" {
			skill.Type = "agent"
		} else {
			skill.Type = "rule"
		}
	}

	return skill, version, nil
}

// bumpVersion increments a semver string.
func bumpVersion(current, bump string) (string, error) {
	var major, minor, patch int
	if _, err := fmt.Sscanf(current, "%d.%d.%d", &major, &minor, &patch); err != nil {
		return "", fmt.Errorf("parse version %q: %w", current, err)
	}

	switch bump {
	case "major":
		major++
		minor = 0
		patch = 0
	case "minor":
		minor++
		patch = 0
	case "patch":
		patch++
	default:
		return "", fmt.Errorf("invalid bump type %q (patch|minor|major)", bump)
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
}
