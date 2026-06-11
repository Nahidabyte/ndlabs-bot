package database

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

type BotSettings struct {
	BotNumber  string
	Owners     string
	Mode       string
	AntiCall   bool
	AutoBlock  bool
	AutoRead   bool
	AutoTyping bool
}

type GroupSettings struct {
	GroupID      string
	IsMuted      bool
	AntiToxic    bool
	AntiLink     bool
	AntiLinkWa   bool
	AntiSpam     bool
	Welcome      bool
	Goodbye      bool
	WelcomeText  string
	GoodbyeText  string
	AutoSticker  bool
	NSFW         bool
	Leveling     bool
	OnlyAdmin    bool
	AntiBot      bool
	AntiDelete   bool
	AntiViewOnce bool
	Simi         bool
	SimiTTS      bool
	MuteOpen     string
	MuteClose    string
	AntiTagSW    bool
	TagSWLimit   int
	AntiNSFW     bool
	AntiJomok    bool
	AntiGay      bool
}

type User struct {
	ID        string
	Name      string
	IsBanned  bool
	IsPremium bool
	Limit     int
	Balance   int
	Level     int
	XP        int
}

func Init(path string) (*DB, error) {

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}

	db.initDefaultSettings()

	return db, nil
}

func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

func (db *DB) migrate() error {
	queries := []string{

		`CREATE TABLE IF NOT EXISTS bot_configs (
			bot_jid TEXT PRIMARY KEY,
			bot_number TEXT DEFAULT '',
			owners TEXT DEFAULT '',
			mode TEXT DEFAULT 'public', -- public, self, group_only, private_only
			anticall BOOLEAN DEFAULT 0,
			autoblock BOOLEAN DEFAULT 0,
			autoread BOOLEAN DEFAULT 0,
			autotyping BOOLEAN DEFAULT 0
		)`,

		`CREATE TABLE IF NOT EXISTS bot_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			bot_number TEXT DEFAULT '',
			owners TEXT DEFAULT '',
			mode TEXT DEFAULT 'public',
			anticall BOOLEAN DEFAULT 0,
			autoblock BOOLEAN DEFAULT 0,
			autoread BOOLEAN DEFAULT 0,
			autotyping BOOLEAN DEFAULT 0
		)`,

		`CREATE TABLE IF NOT EXISTS groups (
			id TEXT PRIMARY KEY,
			is_muted BOOLEAN DEFAULT 0,
			anti_toxic BOOLEAN DEFAULT 0,
			anti_link BOOLEAN DEFAULT 0,
			anti_link_wa BOOLEAN DEFAULT 0,
			anti_spam BOOLEAN DEFAULT 0,
			welcome BOOLEAN DEFAULT 0,
			goodbye BOOLEAN DEFAULT 0,
			welcome_text TEXT DEFAULT 'Selamat datang @user di grup @group!',
			goodbye_text TEXT DEFAULT 'Selamat tinggal @user!',
			auto_sticker BOOLEAN DEFAULT 0,
			nsfw BOOLEAN DEFAULT 0,
			leveling BOOLEAN DEFAULT 0,
			only_admin BOOLEAN DEFAULT 0,
			anti_bot BOOLEAN DEFAULT 0,
			anti_delete BOOLEAN DEFAULT 0,
			anti_viewonce BOOLEAN DEFAULT 0,
			simi BOOLEAN DEFAULT 0,
			simi_tts BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT DEFAULT '',
			is_banned BOOLEAN DEFAULT 0,
			is_premium BOOLEAN DEFAULT 0,
			user_limit INTEGER DEFAULT 100,
			balance INTEGER DEFAULT 0,
			level INTEGER DEFAULT 1,
			xp INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			from_jid TEXT,
			to_jid TEXT,
			text TEXT,
			is_group BOOLEAN,
			group_id TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS warnings (
			group_id TEXT,
			user_id TEXT,
			count INTEGER DEFAULT 0,
			PRIMARY KEY (group_id, user_id)
		)`,

		`CREATE TABLE IF NOT EXISTS ai_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT,
			role TEXT,
			text TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS bot_commands (
			name TEXT PRIMARY KEY,
			category TEXT,
			description TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS ai_memory (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			text TEXT,
			embedding BLOB,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		if _, err := db.conn.Exec(query); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN sewa_until DATETIME")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN sewa_type TEXT DEFAULT 'basic'")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN mute_open TEXT DEFAULT ''")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN mute_close TEXT DEFAULT ''")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN anti_tagsw BOOLEAN DEFAULT 0")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN tagsw_limit INTEGER DEFAULT 2")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN anti_nsfw BOOLEAN DEFAULT 0")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN anti_jomok BOOLEAN DEFAULT 0")
	_, _ = db.conn.Exec("ALTER TABLE groups ADD COLUMN anti_gay BOOLEAN DEFAULT 0")
	_, _ = db.conn.Exec("ALTER TABLE users ADD COLUMN premium_until DATETIME")
	_, _ = db.conn.Exec("ALTER TABLE users ADD COLUMN last_limit_reset DATE DEFAULT CURRENT_DATE")

	_, _ = db.conn.Exec(`CREATE TABLE IF NOT EXISTS tagsw_usage (
		user_id TEXT,
		group_id TEXT,
		date DATE,
		count INTEGER DEFAULT 0,
		PRIMARY KEY (user_id, group_id, date)
	)`)

	return nil
}

func (db *DB) initDefaultSettings() {
	_, _ = db.conn.Exec(`
		INSERT OR IGNORE INTO bot_settings (id, mode, anticall, autoblock)
		VALUES (1, 'public', 0, 0)
	`)

	_, _ = db.conn.Exec(`
		INSERT OR IGNORE INTO bot_configs (bot_jid, bot_number, owners, mode, anticall, autoblock, autoread, autotyping)
		SELECT 'main', bot_number, owners, mode, anticall, autoblock, autoread, autotyping FROM bot_settings WHERE id = 1
	`)
}

func (db *DB) GetBotSettings(botJID string) (*BotSettings, error) {
	if botJID == "" {
		botJID = "main"
	}

	_, _ = db.conn.Exec("INSERT OR IGNORE INTO bot_configs (bot_jid) VALUES (?)", botJID)

	row := db.conn.QueryRow("SELECT bot_number, owners, mode, anticall, autoblock, autoread, autotyping FROM bot_configs WHERE bot_jid = ?", botJID)

	var s BotSettings
	err := row.Scan(&s.BotNumber, &s.Owners, &s.Mode, &s.AntiCall, &s.AutoBlock, &s.AutoRead, &s.AutoTyping)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) UpdateBotMode(botJID string, mode string) error {
	if botJID == "" {
		botJID = "main"
	}
	_, _ = db.conn.Exec("INSERT OR IGNORE INTO bot_configs (bot_jid) VALUES (?)", botJID)
	_, err := db.conn.Exec("UPDATE bot_configs SET mode = ? WHERE bot_jid = ?", mode, botJID)
	return err
}

func (db *DB) GetGroupSettings(groupID string) (*GroupSettings, error) {

	_, _ = db.conn.Exec("INSERT OR IGNORE INTO groups (id) VALUES (?)", groupID)

	row := db.conn.QueryRow(`
		SELECT id, is_muted, anti_toxic, anti_link, anti_link_wa, anti_spam, welcome, goodbye,
		welcome_text, goodbye_text, auto_sticker, nsfw, leveling, only_admin, anti_bot,
		anti_delete, anti_viewonce, simi, simi_tts, mute_open, mute_close, anti_tagsw, tagsw_limit,
		anti_nsfw, anti_jomok, anti_gay
		FROM groups WHERE id = ?`, groupID)

	var g GroupSettings
	err := row.Scan(
		&g.GroupID, &g.IsMuted, &g.AntiToxic, &g.AntiLink, &g.AntiLinkWa, &g.AntiSpam, &g.Welcome, &g.Goodbye,
		&g.WelcomeText, &g.GoodbyeText, &g.AutoSticker, &g.NSFW, &g.Leveling, &g.OnlyAdmin, &g.AntiBot,
		&g.AntiDelete, &g.AntiViewOnce, &g.Simi, &g.SimiTTS, &g.MuteOpen, &g.MuteClose, &g.AntiTagSW, &g.TagSWLimit,
		&g.AntiNSFW, &g.AntiJomok, &g.AntiGay,
	)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (db *DB) UpdateGroupToggle(groupID string, column string, value bool) error {

	query := fmt.Sprintf("UPDATE groups SET %s = ? WHERE id = ?", column)
	_, err := db.conn.Exec(query, value, groupID)
	return err
}

func (db *DB) AddUser(userID string) error {
	_, err := db.conn.Exec(
		"INSERT OR IGNORE INTO users (id, user_limit, last_limit_reset) VALUES (?, 100, CURRENT_DATE)",
		userID,
	)
	return err
}

func (db *DB) GetUser(userID string) (*User, error) {

	db.AddUser(userID)

	_, _ = db.conn.Exec("UPDATE users SET user_limit = 100, last_limit_reset = CURRENT_DATE WHERE id = ? AND (last_limit_reset != CURRENT_DATE OR last_limit_reset IS NULL)", userID)

	row := db.conn.QueryRow("SELECT id, name, is_banned, is_premium, user_limit, balance, level, xp FROM users WHERE id = ?", userID)

	var u User
	err := row.Scan(&u.ID, &u.Name, &u.IsBanned, &u.IsPremium, &u.Limit, &u.Balance, &u.Level, &u.XP)
	if err != nil {
		return nil, err
	}

	var premiumUntil sql.NullTime
	_ = db.conn.QueryRow("SELECT premium_until FROM users WHERE id = ?", userID).Scan(&premiumUntil)
	if premiumUntil.Valid {
		if time.Now().After(premiumUntil.Time) {

			_, _ = db.conn.Exec("UPDATE users SET is_premium = 0, premium_until = NULL WHERE id = ?", userID)
			u.IsPremium = false
		} else {
			u.IsPremium = true
		}
	} else if u.IsPremium {
		_, _ = db.conn.Exec("UPDATE users SET is_premium = 0 WHERE id = ?", userID)
		u.IsPremium = false
	}

	return &u, nil
}

func (db *DB) UpdateUserName(userID, name string) error {
	_, err := db.conn.Exec("UPDATE users SET name = ? WHERE id = ?", name, userID)
	return err
}

func (db *DB) UpdateUserXP(userID string, addXP int) error {
	_, err := db.conn.Exec("UPDATE users SET xp = xp + ? WHERE id = ?", addXP, userID)
	return err
}

func (db *DB) UpdateUserBalance(userID string, amount int) error {
	_, err := db.conn.Exec("UPDATE users SET balance = balance + ? WHERE id = ?", amount, userID)
	return err
}

func (db *DB) AddUserXP(userID string, addXP int) (bool, int, error) {
	user, err := db.GetUser(userID)
	if err != nil {
		return false, 0, err
	}

	newXP := user.XP + addXP
	reqXP := user.Level * 100

	levelUp := false
	newLevel := user.Level

	for newXP >= reqXP {
		newXP -= reqXP
		newLevel++
		reqXP = newLevel * 100
		levelUp = true
	}

	_, err = db.conn.Exec("UPDATE users SET xp = ?, level = ? WHERE id = ?", newXP, newLevel, userID)
	return levelUp, newLevel, err
}

func (db *DB) ReduceUserLimit(userID string, amount int) error {
	_, err := db.conn.Exec("UPDATE users SET user_limit = user_limit - ? WHERE id = ?", amount, userID)
	return err
}

func (db *DB) AddUserPremium(userID string, days int) error {
	db.AddUser(userID)
	_, err := db.conn.Exec(`
		UPDATE users 
		SET is_premium = 1, premium_until = CASE 
			WHEN premium_until IS NULL OR premium_until < CURRENT_TIMESTAMP THEN datetime('now', ?) 
			ELSE datetime(premium_until, ?) 
		END 
		WHERE id = ?`,
		fmt.Sprintf("+%d days", days),
		fmt.Sprintf("+%d days", days),
		userID,
	)
	return err
}

func (db *DB) AddGroupSewa(groupID string, days int, sewaType string) error {
	_, _ = db.conn.Exec("INSERT OR IGNORE INTO groups (id) VALUES (?)", groupID)
	_, err := db.conn.Exec(`
		UPDATE groups 
		SET sewa_type = ?, sewa_until = CASE 
			WHEN sewa_until IS NULL OR sewa_until < CURRENT_TIMESTAMP THEN datetime('now', ?) 
			ELSE datetime(sewa_until, ?) 
		END 
		WHERE id = ?`,
		sewaType,
		fmt.Sprintf("+%d days", days),
		fmt.Sprintf("+%d days", days),
		groupID,
	)
	return err
}

func (db *DB) SaveMessage(id, fromJID, toJID, text string, isGroup bool, groupID string) error {
	_, err := db.conn.Exec(
		"INSERT INTO messages (id, from_jid, to_jid, text, is_group, group_id) VALUES (?, ?, ?, ?, ?, ?)",
		id, fromJID, toJID, text, isGroup, groupID,
	)
	return err
}

func (db *DB) GetWarnings(groupID, userID string) int {
	var count int
	err := db.conn.QueryRow("SELECT count FROM warnings WHERE group_id = ? AND user_id = ?", groupID, userID).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

func (db *DB) AddWarning(groupID, userID string) int {
	count := db.GetWarnings(groupID, userID) + 1
	_, _ = db.conn.Exec("INSERT INTO warnings (group_id, user_id, count) VALUES (?, ?, ?) ON CONFLICT(group_id, user_id) DO UPDATE SET count = ?", groupID, userID, count, count)
	return count
}

func (db *DB) ResetWarnings(groupID, userID string) error {
	_, err := db.conn.Exec("DELETE FROM warnings WHERE group_id = ? AND user_id = ?", groupID, userID)
	return err
}

func (db *DB) ResetAllWarnings(groupID string) error {
	_, err := db.conn.Exec("DELETE FROM warnings WHERE group_id = ?", groupID)
	return err
}

func (db *DB) GetTopUsers(limit int) ([]User, error) {
	rows, err := db.conn.Query("SELECT id, name, is_banned, is_premium, user_limit, balance, level, xp FROM users ORDER BY level DESC, xp DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.IsBanned, &u.IsPremium, &u.Limit, &u.Balance, &u.Level, &u.XP); err == nil {
			users = append(users, u)
		}
	}
	return users, nil
}

func (db *DB) GetTopUsersLocal(jids []string, limit int) ([]User, error) {
	if len(jids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(jids))
	args := make([]interface{}, len(jids))
	for i, jid := range jids {
		placeholders[i] = "?"
		args[i] = jid
	}
	args = append(args, limit)

	query := fmt.Sprintf("SELECT id, name, is_banned, is_premium, user_limit, balance, level, xp FROM users WHERE id IN (%s) ORDER BY level DESC, xp DESC LIMIT ?", strings.Join(placeholders, ","))
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.IsBanned, &u.IsPremium, &u.Limit, &u.Balance, &u.Level, &u.XP); err == nil {
			users = append(users, u)
		}
	}
	return users, nil
}

func (db *DB) AddAIHistory(userID, role, text string) error {
	_, err := db.conn.Exec("INSERT INTO ai_history (user_id, role, text) VALUES (?, ?, ?)", userID, role, text)
	return err
}

type AIHistory struct {
	Role string
	Text string
}

func (db *DB) GetAIHistory(userID string, limit int) ([]AIHistory, error) {
	rows, err := db.conn.Query("SELECT role, text FROM (SELECT role, text, id FROM ai_history WHERE user_id = ? ORDER BY id DESC LIMIT ?) ORDER BY id ASC", userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []AIHistory
	for rows.Next() {
		var role, text string
		if err := rows.Scan(&role, &text); err == nil {
			history = append(history, AIHistory{Role: role, Text: text})
		}
	}
	return history, nil
}

type CommandInfo struct {
	Name        string
	Category    string
	Description string
}

func (db *DB) SyncCommands(cmds []CommandInfo) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}

	_, _ = tx.Exec("DELETE FROM bot_commands")

	stmt, err := tx.Prepare("INSERT INTO bot_commands (name, category, description) VALUES (?, ?, ?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, cmd := range cmds {
		_, err = stmt.Exec(cmd.Name, cmd.Category, cmd.Description)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) SearchRelevantCommands(query string, limit int) ([]CommandInfo, error) {
	words := strings.Fields(query)
	queryParts := make([]string, 0, len(words))
	args := make([]interface{}, 0, len(words)*3)

	for _, w := range words {
		if len(w) <= 2 || strings.ToLower(w) == "tolong" || strings.ToLower(w) == "killua" {
			continue
		}
		likeStr := "%" + w + "%"
		queryParts = append(queryParts, "(name LIKE ? OR category LIKE ? OR description LIKE ?)")
		args = append(args, likeStr, likeStr, likeStr)
	}

	sqlQuery := "SELECT name, category, description FROM bot_commands LIMIT ?"
	if len(queryParts) > 0 {
		sqlQuery = "SELECT name, category, description FROM bot_commands WHERE " + strings.Join(queryParts, " OR ") + " LIMIT ?"
	}
	args = append(args, limit)

	rows, err := db.conn.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CommandInfo
	for rows.Next() {
		var c CommandInfo
		if err := rows.Scan(&c.Name, &c.Category, &c.Description); err == nil {
			results = append(results, c)
		}
	}

	return results, nil
}

func (db *DB) GetConnection() *sql.DB {
	return db.conn
}

func (db *DB) AddTagSWUsage(userID, groupID string) int {
	date := time.Now().Format("2006-01-02")
	var count int
	err := db.conn.QueryRow("SELECT count FROM tagsw_usage WHERE user_id = ? AND group_id = ? AND date = ?", userID, groupID, date).Scan(&count)
	if err != nil {
		count = 0
	}
	count++
	_, _ = db.conn.Exec("INSERT INTO tagsw_usage (user_id, group_id, date, count) VALUES (?, ?, ?, ?) ON CONFLICT(user_id, group_id, date) DO UPDATE SET count = ?", userID, groupID, date, count, count)
	return count
}

func (db *DB) ClearAIHistory(userID string) error {
	_, err := db.conn.Exec("DELETE FROM ai_history WHERE user_id = ?", userID)
	return err
}

type MemoryResult struct {
	Text  string
	Score float64
}

func embToBlob(emb []float32) []byte {
	buf := make([]byte, 4*len(emb))
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func blobToEmb(b []byte) []float32 {
	n := len(b) / 4
	emb := make([]float32, n)
	for i := 0; i < n; i++ {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return emb
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func (db *DB) AddMemory(sessionID, text string, emb []float32) error {
	if strings.TrimSpace(text) == "" || len(emb) == 0 {
		return nil
	}
	_, err := db.conn.Exec(
		"INSERT INTO ai_memory (session_id, text, embedding) VALUES (?, ?, ?)",
		sessionID, text, embToBlob(emb),
	)
	return err
}

const GlobalKnowledgeSession = "__knowledge__"

func (db *DB) HasGlobalKnowledge() bool {
	var count int
	db.conn.QueryRow("SELECT COUNT(*) FROM ai_memory WHERE session_id = ?", GlobalKnowledgeSession).Scan(&count)
	return count > 0
}

func (db *DB) SearchMemory(sessionID string, queryEmb []float32, topK int, minScore float64) ([]MemoryResult, error) {
	if len(queryEmb) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var results []MemoryResult

	scanRows := func(query string, args ...any) {
		rows, err := db.conn.Query(query, args...)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var text string
			var blob []byte
			if err := rows.Scan(&text, &blob); err != nil {
				continue
			}
			if seen[text] {
				continue
			}
			seen[text] = true
			score := cosineSim(queryEmb, blobToEmb(blob))
			if score >= minScore {
				results = append(results, MemoryResult{Text: text, Score: score})
			}
		}
	}

	scanRows("SELECT text, embedding FROM ai_memory WHERE session_id = ? ORDER BY id DESC LIMIT 2000", sessionID)

	if sessionID != GlobalKnowledgeSession {
		scanRows("SELECT text, embedding FROM ai_memory WHERE session_id = ? ORDER BY id ASC", GlobalKnowledgeSession)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (db *DB) ClearMemory(sessionID string) error {
	_, err := db.conn.Exec("DELETE FROM ai_memory WHERE session_id = ?", sessionID)
	return err
}
