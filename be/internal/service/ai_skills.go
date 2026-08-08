package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
)

// newChapterSkillsMiddleware 初始化章节写作技能中间件，供章级助手按需加载本地写作技能。
func newChapterSkillsMiddleware(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	return newSkillsMiddleware(ctx, "chapter-skills", "章节技能")
}

// newStoryEditSkillsMiddleware 初始化故事编辑技能中间件，供 AI 消痕助手加载编辑技能。
func newStoryEditSkillsMiddleware(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	return newSkillsMiddleware(ctx, "story-edit-skills", "故事编辑技能")
}

// newSkillsMiddleware 按技能子目录创建 Eino skill middleware，避免业务 Agent 自己读取 SKILL.md。
func newSkillsMiddleware(ctx context.Context, subdir string, label string) (adk.ChatModelAgentMiddleware, error) {
	skillsDir, err := resolveAISkillsDir()
	if err != nil {
		return nil, err
	}
	skillDir := filepath.Join(skillsDir, subdir)

	fsBackend, err := local.NewBackend(ctx, &local.Config{})
	if err != nil {
		return nil, fmt.Errorf("初始化本地技能后端失败: %w", err)
	}

	chapterSkillsBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: fsBackend,
		BaseDir: skillDir,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化%s目录失败: %w", label, err)
	}

	middleware, err := skill.NewMiddleware(ctx, &skill.Config{
		Backend: chapterSkillsBackend,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化%s中间件失败: %w", label, err)
	}
	return middleware, nil
}

// resolveAISkillsDir 返回运行时可访问的 AI 技能目录。
//
// 优先使用环境变量、可执行文件目录和当前工作目录，只把源码路径作为本地开发兜底。
func resolveAISkillsDir() (string, error) {
	candidates := []string{}
	if configured := os.Getenv("AI_SKILLS_DIR"); configured != "" {
		candidates = append(candidates, configured)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "ai_skills"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "ai_skills"),
			filepath.Join(cwd, "internal", "service", "ai_skills"),
		)
	}

	// runtime.Caller 里的源码路径是编译时写入二进制的，不能作为服务器运行路径。
	// 例如在 Windows 编译后部署到 Linux，runtime.Caller 仍可能返回 D:/...。
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(thisFile), "ai_skills"))
	}
	for _, candidate := range candidates {
		if isDir(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("AI 技能目录不存在，请设置 AI_SKILLS_DIR 或将 ai_skills 放到服务可执行文件同级目录")
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
