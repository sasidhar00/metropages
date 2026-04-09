package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"
	"os"
	"crypto/sha256"
	"encoding/hex"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/crypto/bcrypt"
	
	"github.com/joho/godotenv"
)

type Post struct {
	ID         int
	User       string
	Handle     string
	Content    string
	Price      string
	Category   string
	Tags       string
	ImageURL   string
	Likes      int
	IsBoosted  bool
	IsFeatured bool
	CreatedAt  time.Time
	UserID     int
	Status     string
}

type User struct {
	ID            int
	Username      string
	Handle        string
	Email         string
	Phone         string
	IsAdmin       bool
	Credits       int
	IsPremium     bool
	IsActive      bool
	PremiumUntil  *time.Time
	MembershipTier string
}

type NewsItem struct {
	ID        int
	Title     string
	Content   string
	Timestamp string
	Category  string
	Likes     int
}

type MarketTrend struct {
	ID          int
	Title       string
	Description string
	Trend       string
	Percentage  string
	Category    string
}

type PasswordReset struct {
	ID        int
	Email     string
	Token     string
	ExpiresAt time.Time
	Used      bool
}

var db *sql.DB

func initDB() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ No .env file found, using environment variables")
	}

	// Get Turso credentials
	dbURL := os.Getenv("TURSO_DB_URL")
	authToken := os.Getenv("TURSO_AUTH_TOKEN")

	var err error
	
	if dbURL != "" && authToken != "" {
		log.Println("🔗 Connecting to Turso database...")
		db, err = sql.Open("libsql", dbURL+"?authToken="+authToken)
		if err != nil {
			log.Fatal("Error connecting to Turso:", err)
		}
		log.Println("✅ Connected to Turso successfully!")
	} else {
		log.Println("📁 Using SQLite for local development")
		db, err = sql.Open("sqlite3", "./metropages.db")
		if err != nil {
			log.Fatal("Error opening SQLite:", err)
		}
	}

	if err := db.Ping(); err != nil {
		log.Fatal("Database ping failed:", err)
	}

	// Clean SQL without comments
	createTablesSQL := `
CREATE TABLE IF NOT EXISTS posts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER DEFAULT 0,
    username TEXT,
    handle TEXT,
    content TEXT,
    price TEXT,
    category TEXT,
    tags TEXT,
    image_url TEXT,
    likes INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending',
    boost_until DATETIME,
    featured_until DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE,
    handle TEXT UNIQUE,
    email TEXT UNIQUE,
    phone TEXT DEFAULT '',
    password_hash TEXT,
    is_admin BOOLEAN DEFAULT FALSE,
    credits INTEGER DEFAULT 500,
    is_premium BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    premium_until DATETIME,
    membership_tier TEXT DEFAULT 'free'
);

CREATE TABLE IF NOT EXISTS news (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT,
    content TEXT,
    category TEXT,
    likes INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS market_trends (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT,
    description TEXT,
    trend TEXT,
    percentage TEXT,
    category TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    amount INTEGER,
    type TEXT,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS password_resets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT,
    token TEXT,
    expires_at DATETIME,
    used BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

	_, err = db.Exec(createTablesSQL)
	if err != nil {
		log.Fatal("Error creating tables:", err)
	}
	log.Println("✅ Tables created/verified")

	// Auto-migrate: Add any missing columns (safe to run)
	db.Exec(`ALTER TABLE users ADD COLUMN phone TEXT DEFAULT ''`)
	db.Exec(`ALTER TABLE users ADD COLUMN is_active BOOLEAN DEFAULT TRUE`)

	// Seed default admin
	var adminCount int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&adminCount)
	if adminCount == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		_, err = db.Exec(`INSERT INTO users (username, handle, email, phone, password_hash, is_admin, credits, is_premium, membership_tier) 
		         VALUES (?,?,?,?,?,?,?,?,?)`, 
			"MetroPages Admin", "metropages", "admin@metropages.com", "9999999999", hash, true, 9999, true, "premium")
		if err != nil {
			log.Println("Warning: Could not create admin user:", err)
		} else {
			log.Println("✅ Admin created: admin@metropages.com / admin123")
		}
	}

	// Seed news if empty
	var newsCount int
	db.QueryRow("SELECT COUNT(*) FROM news").Scan(&newsCount)
	if newsCount == 0 {
		newsData := []struct {
			title, content, category string
		}{
			{"🏠 Real Estate Boom", "Property prices up 15% in Tellapur region this quarter.", "real-estate"},
			{"💼 Gig Economy Update", "Remote work opportunities increased by 40% in 2025.", "gig"},
			{"📊 Market Trend", "HMDA approvals for new projects hit all-time high.", "real-estate"},
			{"⚡ Quick Hire", "Top companies now hiring through metropages platform.", "gig"},
			{"🏗️ New Launch", "Luxury villas launching next week in Zaheerabad.", "real-estate"},
		}
		for _, news := range newsData {
			db.Exec("INSERT INTO news (title, content, category) VALUES (?, ?, ?)", news.title, news.content, news.category)
		}
		log.Println("✅ Seeded news articles")
	}

	// Seed market trends if empty
	var trendsCount int
	db.QueryRow("SELECT COUNT(*) FROM market_trends").Scan(&trendsCount)
	if trendsCount == 0 {
		trendsData := []struct {
			title, description, trend, percentage, category string
		}{
			{"Zaheerabad Farms", "Premium plotted development", "up", "+23%", "real-estate"},
			{"Remote Work", "Work from home opportunities", "up", "+45%", "gig"},
			{"Full-time Development", "Software developers in demand", "up", "+32%", "gig"},
			{"Commercial Space", "Office space demand rising", "up", "+18%", "real-estate"},
			{"Freelance Economy", "Gig workers shifting to freelance", "stable", "0%", "gig"},
		}
		for _, trend := range trendsData {
			db.Exec("INSERT INTO market_trends (title, description, trend, percentage, category) VALUES (?, ?, ?, ?, ?)",
				trend.title, trend.description, trend.trend, trend.percentage, trend.category)
		}
		log.Println("✅ Seeded market trends")
	}
}

func generateTagHTML(tags string) template.HTML {
	if tags == "" {
		return ""
	}
	parts := strings.Split(tags, ",")
	var sb strings.Builder
	for _, t := range parts {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		color := "slate"
		switch t {
		case "HMDA", "DTCP", "RERA":
			color = "purple"
		case "Ready to Occupy", "Under Construction":
			color = "emerald"
		case "Remote", "Work from Home":
			color = "blue"
		case "Urgent Hiring", "Immediate Join":
			color = "red"
		case "Flexible Hours":
			color = "amber"
		case "Experienced", "Fresher":
			color = "indigo"
		}
		sb.WriteString(fmt.Sprintf(`<span class="inline-flex items-center px-3 py-1 text-xs font-bold rounded-2xl bg-%s-100 text-%s-700">%s</span>`, color, color, t))
	}
	return template.HTML(sb.String())
}

func getCurrentUser(c *fiber.Ctx) *User {
	cookie := c.Cookies("auth")
	if cookie == "" {
		return nil
	}
	var userID int
	fmt.Sscanf(cookie, "%d", &userID)
	if userID == 0 {
		return nil
	}

	var u User
	var premiumUntil sql.NullTime
	err := db.QueryRow("SELECT id, username, handle, email, COALESCE(phone, ''), is_admin, credits, is_premium, COALESCE(is_active, 1), premium_until, membership_tier FROM users WHERE id = ?", userID).
		Scan(&u.ID, &u.Username, &u.Handle, &u.Email, &u.Phone, &u.IsAdmin, &u.Credits, &u.IsPremium, &u.IsActive, &premiumUntil, &u.MembershipTier)
	
	if err != nil {
		return nil
	}
	
	if !u.IsActive {
		return nil
	}
	
	if premiumUntil.Valid && premiumUntil.Time.After(time.Now()) {
		u.IsPremium = true
		u.PremiumUntil = &premiumUntil.Time
	} else if u.IsPremium {
		db.Exec("UPDATE users SET is_premium = false, membership_tier = 'free' WHERE id = ?", u.ID)
		u.IsPremium = false
	}
	
	return &u
}

func getAllNews() []NewsItem {
	rows, err := db.Query("SELECT id, title, content, category, likes, created_at FROM news ORDER BY created_at DESC")
	if err != nil {
		log.Println("Error fetching all news:", err)
		return []NewsItem{}
	}
	defer rows.Close()

	var newsList []NewsItem
	for rows.Next() {
		var n NewsItem
		var createdAt time.Time
		rows.Scan(&n.ID, &n.Title, &n.Content, &n.Category, &n.Likes, &createdAt)
		diff := time.Since(createdAt)
		if diff < time.Hour {
			n.Timestamp = fmt.Sprintf("%d minutes ago", int(diff.Minutes()))
		} else if diff < 24*time.Hour {
			n.Timestamp = fmt.Sprintf("%d hours ago", int(diff.Hours()))
		} else {
			n.Timestamp = fmt.Sprintf("%d days ago", int(diff.Hours()/24))
		}
		newsList = append(newsList, n)
	}
	return newsList
}

func getMarketTrends() []MarketTrend {
	rows, err := db.Query("SELECT id, title, description, trend, percentage, category FROM market_trends ORDER BY created_at DESC")
	if err != nil {
		log.Println("Error fetching market trends:", err)
		return []MarketTrend{}
	}
	defer rows.Close()

	var trends []MarketTrend
	for rows.Next() {
		var t MarketTrend
		rows.Scan(&t.ID, &t.Title, &t.Description, &t.Trend, &t.Percentage, &t.Category)
		trends = append(trends, t)
	}
	return trends
}

func detectTags(content, category string) string {
	var detectedTags []string
	
	reKeywords := []string{"HMDA", "DTCP", "RERA", "Ready to Occupy", "Under Construction", "Clear Title", "Premium", "Luxury"}
	gigKeywords := []string{"Remote", "Work from Home", "Full-time", "Part-time", "Freelance", "Internship", "Contract", 
		"Urgent Hiring", "Flexible Hours", "Immediate Join", "Experienced", "Fresher"}
	
	keywords := reKeywords
	if category == "Gig" {
		keywords = gigKeywords
	}
	
	for _, kw := range keywords {
		if strings.Contains(strings.ToUpper(content), strings.ToUpper(kw)) {
			detectedTags = append(detectedTags, kw)
		}
	}
	
	return strings.Join(detectedTags, ",")
}

func fetchPosts(query string, args ...interface{}) ([]Post, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		var boosted, featured int
		var createdAt time.Time
		err := rows.Scan(&p.ID, &p.User, &p.Handle, &p.Content, &p.Price, &p.Category, &p.Tags, &p.ImageURL, &p.Likes, &createdAt, &boosted, &featured, &p.UserID, &p.Status)
		if err != nil {
			continue
		}
		p.IsBoosted = boosted == 1
		p.IsFeatured = featured == 1
		p.CreatedAt = createdAt
		posts = append(posts, p)
	}
	return posts, nil
}

func main() {
	initDB()
	defer db.Close()

	engine := html.New("./views", ".html")
	engine.AddFunc("tagHTML", generateTagHTML)

	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New())

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ====================== SEED ======================
	app.Get("/seed", func(c *fiber.Ctx) error {
		db.Exec("DELETE FROM posts")
		
		dummyData := []struct {
			user, handle, content, price, category, tags, image string
			boosted, featured bool
			userID int
		}{
			{"Green Villas", "greenvillas", "3 BHK Premium Villa in Tellapur with HMDA approval.", "₹1.85 Cr", "Real Estate", "HMDA,Ready to Occupy", "https://picsum.photos/id/1015/800/450", false, true, 1},
			{"Swift Logistics", "swiftlogistics", "Need 2 experienced drivers. Full-time position.", "₹35,000/month", "Gig", "Full-time,Urgent Hiring", "https://picsum.photos/id/201/800/450", true, false, 1},
			{"Luxury Plots", "plotking", "DTCP approved plots in Zaheerabad Farms.", "₹45,000/sqyd", "Real Estate", "DTCP,Premium", "https://picsum.photos/id/133/800/450", false, false, 1},
			{"Modern Interiors", "interiorshub", "Modular kitchen installation. Work from home consultation.", "Contact Owner", "Gig", "Work from Home", "https://picsum.photos/id/180/800/450", false, false, 1},
		}

		for _, d := range dummyData {
			query := `INSERT INTO posts (user_id, username, handle, content, price, category, tags, image_url, status`
			args := []interface{}{d.userID, d.user, d.handle, d.content, d.price, d.category, d.tags, d.image, "approved"}
			
			if d.boosted {
				query += ", boost_until"
				args = append(args, time.Now().Add(7*24*time.Hour))
			}
			if d.featured {
				query += ", featured_until"
				args = append(args, time.Now().Add(24*time.Hour))
			}
			query += ") VALUES (" + strings.Repeat("?,", len(args)) + "?)"
			query = strings.Replace(query, ",?)", ")", 1)
			
			db.Exec(query, args...)
		}
		return c.SendString("✅ Dummy data loaded! <a href='/'>Go Home</a>")
	})

	// ====================== HOME ======================
	app.Get("/", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		marketTrends := getMarketTrends()

		posts, _ := fetchPosts(`
			SELECT id, username, handle, content, price, category, tags, image_url, likes, created_at,
			       CASE WHEN boost_until > datetime('now') THEN 1 ELSE 0 END,
			       CASE WHEN featured_until > datetime('now') THEN 1 ELSE 0 END,
			       user_id, status
			FROM posts 
			WHERE status = 'approved' 
			ORDER BY featured_until DESC, boost_until DESC, id DESC
		`)

		return c.Render("index", fiber.Map{
			"Posts":        posts,
			"CurrentUser":  currentUser,
			"IsProfile":    false,
			"SearchQuery":  "",
			"IsAdmin":      currentUser != nil && currentUser.IsAdmin,
			"MarketTrends": marketTrends,
		})
	})

	// ====================== NEWS PAGE ======================
	app.Get("/news", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		allNews := getAllNews()

		return c.Render("news", fiber.Map{
			"CurrentUser": currentUser,
			"News":        allNews,
			"IsAdmin":     currentUser != nil && currentUser.IsAdmin,
		})
	})

	// Like news
	app.Post("/news/like/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		db.Exec("UPDATE news SET likes = likes + 1 WHERE id = ?", id)
		var likes int
		db.QueryRow("SELECT likes FROM news WHERE id = ?", id).Scan(&likes)
		return c.SendString(fmt.Sprintf("%d", likes))
	})

	// ====================== ADMIN NEWS MANAGEMENT ======================
	app.Post("/admin/news/add", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		title := c.FormValue("title")
		content := c.FormValue("content")
		category := c.FormValue("category")

		if title == "" || content == "" {
			return c.Status(400).SendString("Title and content are required")
		}

		_, err := db.Exec("INSERT INTO news (title, content, category) VALUES (?, ?, ?)", title, content, category)
		if err != nil {
			return c.Status(500).SendString("Error adding news")
		}

		return c.SendString(`✅ News added successfully!<script>window.location.reload()</script>`)
	})

	app.Post("/admin/news/delete/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		id := c.Params("id")
		_, err := db.Exec("DELETE FROM news WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Error deleting news")
		}

		return c.SendString("✅ News deleted")
	})

	app.Post("/admin/news/edit/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		id := c.Params("id")
		title := c.FormValue("title")
		content := c.FormValue("content")
		category := c.FormValue("category")

		_, err := db.Exec("UPDATE news SET title = ?, content = ?, category = ? WHERE id = ?", title, content, category, id)
		if err != nil {
			return c.Status(500).SendString("Error updating news")
		}

		return c.SendString(`✅ News updated!<script>window.location.reload()</script>`)
	})

	// ====================== ADMIN MARKET TRENDS MANAGEMENT ======================
	app.Post("/admin/trends/add", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		title := c.FormValue("title")
		description := c.FormValue("description")
		trend := c.FormValue("trend")
		percentage := c.FormValue("percentage")
		category := c.FormValue("category")

		_, err := db.Exec("INSERT INTO market_trends (title, description, trend, percentage, category) VALUES (?, ?, ?, ?, ?)",
			title, description, trend, percentage, category)
		if err != nil {
			return c.Status(500).SendString("Error adding trend")
		}

		return c.SendString(`✅ Market trend added!<script>window.location.reload()</script>`)
	})

	app.Post("/admin/trends/delete/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		id := c.Params("id")
		_, err := db.Exec("DELETE FROM market_trends WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Error deleting trend")
		}

		return c.SendString("✅ Trend deleted")
	})

	app.Post("/admin/trends/edit/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		id := c.Params("id")
		title := c.FormValue("title")
		description := c.FormValue("description")
		trend := c.FormValue("trend")
		percentage := c.FormValue("percentage")
		category := c.FormValue("category")

		_, err := db.Exec("UPDATE market_trends SET title = ?, description = ?, trend = ?, percentage = ?, category = ? WHERE id = ?",
			title, description, trend, percentage, category, id)
		if err != nil {
			return c.Status(500).SendString("Error updating trend")
		}

		return c.SendString(`✅ Trend updated!<script>window.location.reload()</script>`)
	})

	// ====================== SEARCH ======================
	app.Get("/search", func(c *fiber.Ctx) error {
		query := c.Query("q")
		if query == "" {
			return c.Redirect("/")
		}

		posts, _ := fetchPosts(`
			SELECT id, username, handle, content, price, category, tags, image_url, likes, created_at,
			       CASE WHEN boost_until > datetime('now') THEN 1 ELSE 0 END,
			       CASE WHEN featured_until > datetime('now') THEN 1 ELSE 0 END,
			       user_id, status
			FROM posts 
			WHERE (content LIKE ? OR tags LIKE ? OR category LIKE ?) AND status = 'approved'
			ORDER BY id DESC
		`, "%"+query+"%", "%"+query+"%", "%"+query+"%")

		return c.Render("index", fiber.Map{
			"Posts":        posts,
			"CurrentUser":  getCurrentUser(c),
			"SearchQuery":  query,
			"IsProfile":    false,
			"IsAdmin":      false,
			"MarketTrends": getMarketTrends(),
		})
	})

	// ====================== PROFILE ======================
	app.Get("/profile", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Redirect("/")
		}

		posts, _ := fetchPosts(`
			SELECT id, username, handle, content, price, category, tags, image_url, likes, created_at,
			       CASE WHEN boost_until > datetime('now') THEN 1 ELSE 0 END,
			       CASE WHEN featured_until > datetime('now') THEN 1 ELSE 0 END,
			       user_id, status
			FROM posts 
			WHERE user_id = ? AND status = 'approved'
			ORDER BY id DESC
		`, currentUser.ID)

		return c.Render("index", fiber.Map{
			"Posts":        posts,
			"CurrentUser":  currentUser,
			"IsProfile":    true,
			"SearchQuery":  "",
			"IsAdmin":      currentUser.IsAdmin,
			"MarketTrends": getMarketTrends(),
		})
	})

	// ====================== CREATE POST ======================
	app.Post("/post", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Status(401).SendString(`<div class="p-4 text-red-500">Please login to post</div>`)
		}

		content := c.FormValue("content")
		category := c.FormValue("category")
		imageURL := c.FormValue("image_url")
		price := c.FormValue("price")
		if price == "" {
			price = "Contact Owner"
		}

		if content == "" {
			return c.Status(400).SendString(`<div class="p-4 text-red-500">Content cannot be empty</div>`)
		}

		tagsString := detectTags(content, category)
		
		status := "pending"
		if currentUser.IsAdmin || currentUser.IsPremium {
			status = "approved"
		}

		_, err := db.Exec(`
			INSERT INTO posts (user_id, username, handle, content, price, category, tags, image_url, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, currentUser.ID, currentUser.Username, currentUser.Handle, content, price, category, tagsString, imageURL, status)

		if err != nil {
			log.Println("Error creating post:", err)
			return c.Status(500).SendString(`<div class="p-4 text-red-500">Failed to create post</div>`)
		}

		msg := "✅ Post created successfully!"
		if status == "pending" {
			msg = "📝 Post submitted for admin approval."
		}
		
		return c.SendString(`<div class="p-4 text-emerald-600">` + msg + `</div><script>window.location.reload()</script>`)
	})

	// ====================== FORGOT PASSWORD ======================
	app.Get("/forgot-password", func(c *fiber.Ctx) error {
		return c.Render("forgot-password", fiber.Map{
			"CurrentUser": getCurrentUser(c),
		})
	})

	app.Post("/forgot-password", func(c *fiber.Ctx) error {
		email := c.FormValue("email")
		
		var userID int
		err := db.QueryRow("SELECT id FROM users WHERE email = ? AND is_active = 1", email).Scan(&userID)
		if err != nil {
			return c.SendString(`<div class="p-4 text-emerald-600 text-center">If your email exists, you will receive a reset link.</div>`)
		}
		
		token := fmt.Sprintf("%d_%s", time.Now().UnixNano(), email)
		hash := sha256.Sum256([]byte(token))
		hashToken := hex.EncodeToString(hash[:])
		
		expiresAt := time.Now().Add(1 * time.Hour)
		_, err = db.Exec(`INSERT INTO password_resets (email, token, expires_at) VALUES (?, ?, ?)`, 
			email, hashToken, expiresAt)
		
		if err != nil {
			log.Println("Error saving reset token:", err)
			return c.Status(500).SendString("Internal error")
		}
		
		resetLink := fmt.Sprintf("/reset-password?token=%s", token)
		
		return c.SendString(fmt.Sprintf(`
			<div class="p-4 text-emerald-600 text-center">
				✅ Reset link generated!<br>
				<a href="%s" class="text-[#1d9bf0] underline">Click here to reset your password</a>
				<p class="text-xs text-slate-400 mt-2">(Demo - In production, this would be emailed)</p>
			</div>
		`, resetLink))
	})

	// ====================== RESET PASSWORD ======================
	app.Get("/reset-password", func(c *fiber.Ctx) error {
		token := c.Query("token")
		if token == "" {
			return c.Redirect("/")
		}
		
		hash := sha256.Sum256([]byte(token))
		hashToken := hex.EncodeToString(hash[:])
		
		var email string
		var expiresAt time.Time
		err := db.QueryRow(`SELECT email, expires_at FROM password_resets WHERE token = ? AND used = FALSE AND expires_at > datetime('now')`,
			hashToken).Scan(&email, &expiresAt)
		
		if err != nil {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Invalid or expired reset link.</div>`)
		}
		
		return c.Render("reset-password", fiber.Map{
			"Token": token,
			"Email": email,
		})
	})

	app.Post("/reset-password", func(c *fiber.Ctx) error {
		token := c.FormValue("token")
		password := c.FormValue("password")
		
		if len(password) < 6 {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Password must be at least 6 characters</div>`)
		}
		
		hash := sha256.Sum256([]byte(token))
		hashToken := hex.EncodeToString(hash[:])
		
		var email string
		err := db.QueryRow(`SELECT email FROM password_resets WHERE token = ? AND used = FALSE AND expires_at > datetime('now')`,
			hashToken).Scan(&email)
		
		if err != nil {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Invalid or expired reset link</div>`)
		}
		
		hashPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err = db.Exec("UPDATE users SET password_hash = ? WHERE email = ?", hashPassword, email)
		if err != nil {
			return c.Status(500).SendString("Error updating password")
		}
		
		db.Exec("UPDATE password_resets SET used = TRUE WHERE token = ?", hashToken)
		
		return c.SendString(`<div class="p-4 text-emerald-600 text-center">Password reset successfully! <a href="/" class="text-[#1d9bf0]">Login now</a></div>`)
	})

	// ====================== USER MANAGEMENT API (ADMIN) ======================
	app.Get("/admin/users", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}
		
		rows, err := db.Query(`SELECT id, username, handle, email, COALESCE(phone, ''), is_admin, credits, is_premium, COALESCE(is_active, 1) FROM users ORDER BY id DESC`)
		if err != nil {
			return c.Status(500).SendString("Error fetching users")
		}
		defer rows.Close()
		
		var users []map[string]interface{}
		for rows.Next() {
			var id, credits int
			var username, handle, email, phone string
			var isAdmin, isPremium, isActive bool
			rows.Scan(&id, &username, &handle, &email, &phone, &isAdmin, &credits, &isPremium, &isActive)
			users = append(users, map[string]interface{}{
				"id": id, "username": username, "handle": handle, "email": email, "phone": phone,
				"is_admin": isAdmin, "credits": credits, "is_premium": isPremium, "is_active": isActive,
			})
		}
		
		return c.JSON(users)
	})

	app.Post("/admin/user/update", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}
		
		userID := c.FormValue("user_id")
		credits := c.FormValue("credits")
		isPremium := c.FormValue("is_premium") == "on"
		isActive := c.FormValue("is_active") == "on"
		
		var creditsInt int
		fmt.Sscanf(credits, "%d", &creditsInt)
		
		_, err := db.Exec("UPDATE users SET credits = ?, is_premium = ?, is_active = ? WHERE id = ?", 
			creditsInt, isPremium, isActive, userID)
		if err != nil {
			return c.Status(500).SendString("Error updating user")
		}
		
		return c.SendString("✅ User updated")
	})

	app.Post("/admin/user/delete/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}
		
		id := c.Params("id")
		db.Exec("DELETE FROM users WHERE id = ?", id)
		db.Exec("DELETE FROM posts WHERE user_id = ?", id)
		db.Exec("DELETE FROM comments WHERE user_id = ?", id)
		db.Exec("DELETE FROM applications WHERE applicant_id = ?", id)
		
		return c.SendString("✅ User deleted")
	})

	// ====================== UPGRADE TO PREMIUM ======================
	app.Post("/upgrade", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Status(401).JSON(fiber.Map{
				"success": false,
				"message": "Please login first",
			})
		}

		if currentUser.IsPremium {
			return c.JSON(fiber.Map{
				"success": false,
				"message": "You are already a premium member!",
			})
		}

		cost := 499
		if currentUser.Credits < cost {
			return c.JSON(fiber.Map{
				"success": false,
				"message": fmt.Sprintf("Not enough credits! Need %d credits.", cost),
			})
		}

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error starting transaction",
			})
		}
		defer tx.Rollback()

		_, err = tx.Exec("UPDATE users SET credits = credits - ? WHERE id = ?", cost, currentUser.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error updating credits",
			})
		}

		premiumUntil := time.Now().Add(30 * 24 * time.Hour)
		_, err = tx.Exec("UPDATE users SET is_premium = true, premium_until = ?, membership_tier = 'premium' WHERE id = ?",
			premiumUntil, currentUser.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error activating premium",
			})
		}

		_, err = tx.Exec("UPDATE users SET credits = credits + 100 WHERE id = ?", currentUser.ID)
		if err != nil {
			log.Println("Error adding bonus credits:", err)
		}

		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			currentUser.ID, cost, "debit", "Premium membership upgrade (30 days)")
		if err != nil {
			log.Println("Error logging transaction:", err)
		}

		err = tx.Commit()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error completing upgrade",
			})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "✅ Successfully upgraded to Premium! You received 100 bonus credits.",
		})
	})

	// ====================== BOOST POST ======================
	app.Post("/boost/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Status(401).SendString("Please login")
		}

		cost := 99
		if currentUser.Credits < cost {
			return c.SendString(fmt.Sprintf("Not enough credits! Need %d credits.", cost))
		}

		id := c.Params("id")

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString("Error starting transaction")
		}
		defer tx.Rollback()

		_, err = tx.Exec("UPDATE users SET credits = credits - ? WHERE id = ?", cost, currentUser.ID)
		if err != nil {
			return c.Status(500).SendString("Error updating credits")
		}

		_, err = tx.Exec(`UPDATE posts SET boost_until = datetime('now', '+7 days') WHERE id = ?`, id)
		if err != nil {
			return c.Status(500).SendString("Boost failed")
		}

		err = tx.Commit()
		if err != nil {
			return c.Status(500).SendString("Error committing transaction")
		}

		return c.SendString("✅ Post boosted successfully for 7 days!")
	})

	// ====================== FEATURE POST ======================
	app.Post("/feature/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Status(401).SendString("Please login")
		}

		cost := 199
		if currentUser.Credits < cost {
			return c.SendString(fmt.Sprintf("Not enough credits! Need %d credits.", cost))
		}

		id := c.Params("id")

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString("Error starting transaction")
		}
		defer tx.Rollback()

		_, err = tx.Exec("UPDATE users SET credits = credits - ? WHERE id = ?", cost, currentUser.ID)
		if err != nil {
			return c.Status(500).SendString("Error updating credits")
		}

		_, err = tx.Exec(`UPDATE posts SET featured_until = datetime('now', '+1 day') WHERE id = ?`, id)
		if err != nil {
			return c.Status(500).SendString("Feature failed")
		}

		err = tx.Commit()
		if err != nil {
			return c.Status(500).SendString("Error committing transaction")
		}

		return c.SendString("✅ Post featured for 24 hours!")
	})

	// ====================== BUY CREDITS (Placeholder) ======================
	app.Post("/buy-credits", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Status(401).SendString("Please login")
		}
		return c.SendString(`<div class="p-4 text-amber-600 text-center">🚧 Payment integration coming soon!</div>`)
	})

	// ====================== LIKE POST ======================
	app.Post("/like/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		_, err := db.Exec("UPDATE posts SET likes = likes + 1 WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Error updating likes")
		}
		var likes int
		err = db.QueryRow("SELECT likes FROM posts WHERE id = ?", id).Scan(&likes)
		if err != nil {
			return c.Status(500).SendString("Error fetching likes")
		}
		return c.SendString(fmt.Sprintf(`<button hx-post="/like/%s" hx-swap="outerHTML" class="flex items-center gap-1.5 text-slate-400 hover:text-red-500 transition-all active:scale-110">❤️ <span class="text-sm font-medium">%d</span></button>`, id, likes))
	})

	// ====================== ADMIN DASHBOARD ======================
	app.Get("/admin", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied. Admin only.")
		}

		pendingPosts, _ := fetchPosts(`
			SELECT id, username, handle, content, price, category, tags, image_url, likes, created_at,
			       CASE WHEN boost_until > datetime('now') THEN 1 ELSE 0 END,
			       CASE WHEN featured_until > datetime('now') THEN 1 ELSE 0 END,
			       user_id, status
			FROM posts 
			WHERE status = 'pending'
			ORDER BY created_at DESC
		`)

		allNews := getAllNews()
		marketTrends := getMarketTrends()

		rows, err := db.Query(`
			SELECT id, username, handle, email, COALESCE(phone, ''), is_admin, credits, is_premium, COALESCE(is_active, 1) 
			FROM users ORDER BY id DESC
		`)
		if err != nil {
			log.Println("Error fetching users:", err)
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var u User
			err := rows.Scan(&u.ID, &u.Username, &u.Handle, &u.Email, &u.Phone, &u.IsAdmin, &u.Credits, &u.IsPremium, &u.IsActive)
			if err != nil {
				continue
			}
			users = append(users, u)
		}

		var totalPosts, totalUsers, totalCreditsSpent int
		db.QueryRow("SELECT COUNT(*) FROM posts WHERE status = 'approved'").Scan(&totalPosts)
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&totalUsers)
		db.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE type = 'debit'").Scan(&totalCreditsSpent)

		return c.Render("admin", fiber.Map{
			"CurrentUser":        currentUser,
			"PendingPosts":       pendingPosts,
			"Users":              users,
			"TotalPosts":         totalPosts,
			"TotalUsers":         totalUsers,
			"TotalCreditsSpent":  totalCreditsSpent,
			"AllNews":            allNews,
			"MarketTrends":       marketTrends,
		})
	})

	// Approve post
	app.Post("/admin/approve/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}
		id := c.Params("id")
		db.Exec("UPDATE posts SET status = 'approved' WHERE id = ?", id)
		return c.SendString("✅ Post approved")
	})

	// Delete post
	app.Post("/admin/delete/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}
		id := c.Params("id")
		db.Exec("DELETE FROM posts WHERE id = ?", id)
		return c.SendString("✅ Post deleted")
	})

	// Give credits to user
	app.Post("/admin/give-credits", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}
		userID := c.FormValue("user_id")
		credits := c.FormValue("credits")
		var creditsInt int
		fmt.Sscanf(credits, "%d", &creditsInt)
		db.Exec("UPDATE users SET credits = credits + ? WHERE id = ?", creditsInt, userID)
		db.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			userID, creditsInt, "credit", "Admin grant")
		return c.SendString(fmt.Sprintf("✅ Added %d credits", creditsInt))
	})

	// ====================== AUTH ======================
	app.Post("/login", func(c *fiber.Ctx) error {
		email := c.FormValue("email")
		password := c.FormValue("password")

		var id int
		var username, handle string
		var isAdmin bool
		var isActive bool
		var hash string

		err := db.QueryRow("SELECT id, username, handle, is_admin, is_active, password_hash FROM users WHERE email = ?", email).
			Scan(&id, &username, &handle, &isAdmin, &isActive, &hash)
		if err != nil {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Invalid email or password</div>`)
		}
		
		if !isActive {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Account is deactivated. Please contact admin.</div>`)
		}
		
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Invalid email or password</div>`)
		}

		c.Cookie(&fiber.Cookie{
			Name:  "auth",
			Value: fmt.Sprintf("%d", id),
			Path:  "/",
		})
		return c.SendString(`<div class="p-4 text-emerald-600 text-center">Logged in successfully!<script>window.location.reload()</script></div>`)
	})

	app.Post("/signup", func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		handle := c.FormValue("handle")
		email := c.FormValue("email")
		phone := c.FormValue("phone")
		password := c.FormValue("password")

		if len(password) < 6 {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Password must be at least 6 characters</div>`)
		}

		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := db.Exec(`INSERT INTO users (username, handle, email, phone, password_hash, credits) VALUES (?, ?, ?, ?, ?, ?)`,
			username, handle, email, phone, hash, 500)
		if err != nil {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Handle or email already taken</div>`)
		}

		return c.SendString(`<div class="p-4 text-emerald-600 text-center">Account created successfully!<script>window.location.reload()</script></div>`)
	})

	app.Post("/logout", func(c *fiber.Ctx) error {
		c.ClearCookie("auth")
		return c.SendString(`<div class="p-4 text-center">Logged out successfully<script>window.location.reload()</script></div>`)
	})

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("🚀 Server running on http://localhost:%s", port)
	log.Fatal(app.Listen(":" + port))
}
