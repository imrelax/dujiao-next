package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

const minPasswordLength = 8

func usage() {
	fmt.Fprintln(os.Stderr, `admin-tool: 后台管理员运维工具

用法:
  admin-tool list-admins                            列出所有管理员
  admin-tool reset-2fa --username <name>            重置指定管理员的 2FA
  admin-tool reset-password --username <name> [--password <new>]
                                                     重置管理员密码（超管忘记密码恢复用）
                                                     不传 --password 时从 stdin 隐藏读入两次确认

读取的配置文件与 server 相同（默认 config.yml）。`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd := os.Args[1]

	cfg := config.Load()
	if err := models.InitDB(cfg.Database.Driver, cfg.Database.DSN, models.DBPoolConfig{
		MaxOpenConns:           cfg.Database.Pool.MaxOpenConns,
		MaxIdleConns:           cfg.Database.Pool.MaxIdleConns,
		ConnMaxLifetimeSeconds: cfg.Database.Pool.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: cfg.Database.Pool.ConnMaxIdleTimeSeconds,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "init db: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "list-admins":
		listAdmins()
	case "reset-2fa":
		username := parseStringFlag(os.Args[2:], "username")
		if username == "" {
			fmt.Fprintln(os.Stderr, "missing --username")
			usage()
			os.Exit(1)
		}
		resetTOTP(username)
	case "reset-password":
		username := parseStringFlag(os.Args[2:], "username")
		password := parseStringFlag(os.Args[2:], "password")
		if username == "" {
			fmt.Fprintln(os.Stderr, "missing --username")
			usage()
			os.Exit(1)
		}
		resetPassword(username, password)
	default:
		usage()
		os.Exit(1)
	}
}

// parseStringFlag 解析 --name value 或 --name=value，未出现返回 ""
func parseStringFlag(args []string, name string) string {
	prefix := "--" + name
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == prefix && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, prefix+"=") {
			return a[len(prefix)+1:]
		}
	}
	return ""
}

func listAdmins() {
	repo := repository.NewAdminRepository(models.DB)
	admins, err := repo.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "list: %v\n", err)
		os.Exit(1)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tUSERNAME\tIS_SUPER\t2FA_ENABLED\tLAST_LOGIN")
	for _, a := range admins {
		enabled := "no"
		if a.TOTPEnabledAt != nil {
			enabled = "yes (" + a.TOTPEnabledAt.Format("2006-01-02") + ")"
		}
		last := "-"
		if a.LastLoginAt != nil {
			last = a.LastLoginAt.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%d\t%s\t%t\t%s\t%s\n", a.ID, a.Username, a.IsSuper, enabled, last)
	}
	_ = w.Flush()
}

func resetTOTP(username string) {
	repo := repository.NewAdminRepository(models.DB)
	logRepo := repository.NewAdminLoginLogRepository(models.DB)

	admin, err := repo.GetByUsername(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup: %v\n", err)
		os.Exit(1)
	}
	if admin == nil {
		fmt.Fprintf(os.Stderr, "no admin with username=%q\n", username)
		os.Exit(1)
	}
	if err := repo.ClearTOTP(admin.ID); err != nil {
		fmt.Fprintf(os.Stderr, "clear: %v\n", err)
		os.Exit(1)
	}
	rid := "cli-" + uuid.NewString()
	_ = logRepo.Create(&models.AdminLoginLog{
		AdminID:   admin.ID,
		Username:  admin.Username,
		EventType: constants.AdminLoginEvent2FAResetByAdmin,
		Status:    constants.AdminLoginStatusSuccess,
		ClientIP:  "cli",
		UserAgent: "admin-tool",
		RequestID: rid,
		// OperatorID: nil — CLI 操作没有操作者管理员
	})
	fmt.Printf("OK: 2FA reset for admin id=%d username=%s at %s\n", admin.ID, admin.Username, time.Now().Format(time.RFC3339))
}

func resetPassword(username, providedPassword string) {
	repo := repository.NewAdminRepository(models.DB)
	logRepo := repository.NewAdminLoginLogRepository(models.DB)

	admin, err := repo.GetByUsername(username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup: %v\n", err)
		os.Exit(1)
	}
	if admin == nil {
		fmt.Fprintf(os.Stderr, "no admin with username=%q\n", username)
		os.Exit(1)
	}

	newPassword, err := obtainNewPassword(providedPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash: %v\n", err)
		os.Exit(1)
	}

	if err := repo.UpdatePassword(admin.ID, string(hash)); err != nil {
		fmt.Fprintf(os.Stderr, "update: %v\n", err)
		os.Exit(1)
	}

	rid := "cli-" + uuid.NewString()
	_ = logRepo.Create(&models.AdminLoginLog{
		AdminID:   admin.ID,
		Username:  admin.Username,
		EventType: constants.AdminLoginEventPasswordResetByCLI,
		Status:    constants.AdminLoginStatusSuccess,
		ClientIP:  "cli",
		UserAgent: "admin-tool",
		RequestID: rid,
	})
	fmt.Printf("OK: password reset for admin id=%d username=%s at %s\n", admin.ID, admin.Username, time.Now().Format(time.RFC3339))
	fmt.Println("提示: 该管理员所有现有会话已强制下线，请用新密码重新登录。")
}

// obtainNewPassword 决定新密码来源：
//   - 命令行 --password 直接提供时校验后返回
//   - 否则从 stdin 隐藏读取两次确认
func obtainNewPassword(provided string) (string, error) {
	if provided != "" {
		if err := sanityCheckPassword(provided); err != nil {
			return "", err
		}
		return provided, nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// 非终端环境（pipe / CI）允许从 stdin 读一行明文，方便脚本化
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		pwd := strings.TrimRight(line, "\r\n")
		if err := sanityCheckPassword(pwd); err != nil {
			return "", err
		}
		return pwd, nil
	}

	fmt.Print("新密码: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Print("再次输入: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password confirmation: %w", err)
	}
	if string(first) != string(second) {
		return "", errors.New("两次输入不一致")
	}
	pwd := string(first)
	if err := sanityCheckPassword(pwd); err != nil {
		return "", err
	}
	return pwd, nil
}

// sanityCheckPassword 仅做最低限度校验（长度），CLI 紧急恢复场景不强制
// 后台密码策略；建议运维登录后立即在管理界面改成符合策略的强密码。
func sanityCheckPassword(pwd string) error {
	if pwd == "" {
		return errors.New("密码不能为空")
	}
	if len([]rune(pwd)) < minPasswordLength {
		return fmt.Errorf("密码长度至少 %d 个字符", minPasswordLength)
	}
	return nil
}
