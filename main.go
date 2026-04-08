package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"
    "os"
	"libsql"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html/v2"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tursodatabase/libsql-client-go/libsql"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/crypto/bcrypt"
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
	IsAdmin       bool
	Credits       int
	IsPremium     bool
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

var db *sql.DB

func initDB() {
	// Get Turso credentials from environment variables
	tursoURL := os.Getenv("TURSO_DATABASE_URL")
	tursoAuthToken := os.Getenv("TURSO_AUTH_TOKEN")
	
	if tursoURL == "" {
		log.Fatal("TURSO_DATABASE_URL environment variable is required")
	}
	
	var err error
	// Connect to Turso (libSQL)
	db, err = libsql.Open("libsql", tursoURL+"?authToken="+tursoAuthToken)
	if err != nil {
		log.Fatal("Error connecting to Turso:", err)
	}
	
	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Fatal("Error pinging Turso database:", err)
	}
	log.Println("✅ Connected to Turso database")
	// Create tables
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
			password_hash TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			credits INTEGER DEFAULT 500,
			is_premium BOOLEAN DEFAULT FALSE,
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

		CREATE TABLE IF NOT EXISTS transactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			amount INTEGER,
			type TEXT,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`

	_, err = db.Exec(createTablesSQL)
	if err != nil {
		log.Fatal("Error creating tables:", err)
	}
	log.Println("✅ Tables created/verified")

	// Seed default admin
	var adminCount int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = 1").Scan(&adminCount)
	if adminCount == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		_, err = db.Exec(`INSERT INTO users (username, handle, email, password_hash, is_admin, credits, is_premium, membership_tier) 
		         VALUES (?,?,?,?,?,?,?,?)`, 
			"MetroPages Admin", "metropages", "admin@metropages.com", hash, true, 9999, true, "premium")
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
			{"🏠 Real Estate Boom", "Property prices up 15% in Tellapur region this quarter. Experts predict continued growth.", "real-estate"},
			{"💼 Gig Economy Update", "Remote work opportunities increased by 40% in 2025. New platforms emerging.", "gig"},
			{"📊 Market Trend", "HMDA approvals for new projects hit all-time high. 50+ projects approved.", "real-estate"},
			{"⚡ Quick Hire", "Top companies now hiring through metropages platform. 1000+ jobs available.", "gig"},
			{"🏗️ New Launch", "Luxury villas launching next week in Zaheerabad. Pre-booking opens tomorrow.", "real-estate"},
			{"💻 Tech Hiring", "Software developers in high demand. Salaries up 25% year over year.", "gig"},
			{"🏢 Commercial Space", "Office space demand rising in Hitech City. 30% increase in leases.", "real-estate"},
		}
		for _, news := range newsData {
			db.Exec("INSERT INTO news (title, content, category) VALUES (?, ?, ?)", news.title, news.content, news.category)
		}
		log.Println("✅ Seeded", len(newsData), "news articles")
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
	err := db.QueryRow("SELECT id, username, handle, is_admin, credits, is_premium, premium_until, membership_tier FROM users WHERE id = ?", userID).
		Scan(&u.ID, &u.Username, &u.Handle, &u.IsAdmin, &u.Credits, &u.IsPremium, &premiumUntil, &u.MembershipTier)
	
	if err != nil {
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
			{"Green Villas", "greenvillas", "3 BHK Premium Villa in Tellapur with HMDA approval. Ready to occupy.", "₹1.85 Cr", "Real Estate", "HMDA,Ready to Occupy", "https://picsum.photos/id/1015/800/450", false, true, 1},
			{"Swift Logistics", "swiftlogistics", "Need 2 experienced drivers for NIMZ project. Full-time position with good salary.", "₹35,000/month", "Gig", "Full-time,Urgent Hiring,Experienced", "https://picsum.photos/id/201/800/450", true, false, 1},
			{"Luxury Plots", "plotking", "DTCP approved 200 sqyd plots in Zaheerabad Farms. Premium location.", "₹45,000/sqyd", "Real Estate", "DTCP,Premium,Clear Title", "https://picsum.photos/id/133/800/450", false, false, 1},
			{"Modern Interiors", "interiorshub", "Complete modular kitchen and wardrobe installation. Work from home consultation.", "Contact Owner", "Gig", "Work from Home,Flexible Hours", "https://picsum.photos/id/180/800/450", false, false, 1},
			{"Tech Solutions", "techgigs", "Looking for freelance web developers for e-commerce project. Remote work.", "₹50,000 - ₹80,000", "Gig", "Freelance,Remote,Flexible Hours", "https://picsum.photos/id/0/800/450", false, false, 1},
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
			msg = "📝 Post submitted for admin approval. You'll be notified once approved."
		}
		
		return c.SendString(`<div class="p-4 text-emerald-600">` + msg + `</div><script>window.location.reload()</script>`)
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
				"message": fmt.Sprintf("Not enough credits! Need %d credits. Purchase credits from the store.", cost),
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

		// Deduct credits
		_, err = tx.Exec("UPDATE users SET credits = credits - ? WHERE id = ?", cost, currentUser.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error updating credits",
			})
		}

		// Set premium membership for 30 days
		premiumUntil := time.Now().Add(30 * 24 * time.Hour)
		_, err = tx.Exec("UPDATE users SET is_premium = true, premium_until = ?, membership_tier = 'premium' WHERE id = ?",
			premiumUntil, currentUser.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{
				"success": false,
				"message": "Error activating premium",
			})
		}

		// Add bonus credits for upgrading (100 bonus credits)
		_, err = tx.Exec("UPDATE users SET credits = credits + 100 WHERE id = ?", currentUser.ID)
		if err != nil {
			log.Println("Error adding bonus credits:", err)
		}

		// Log transaction
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			currentUser.ID, cost, "debit", "Premium membership upgrade (30 days)")
		if err != nil {
			log.Println("Error logging transaction:", err)
		}

		// Log bonus
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			currentUser.ID, 100, "credit", "Welcome bonus for upgrading to premium")
		if err != nil {
			log.Println("Error logging bonus:", err)
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
			"message": "✅ Successfully upgraded to Premium! You received 100 bonus credits. Premium benefits active for 30 days.",
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
			return c.SendString(fmt.Sprintf("Not enough credits! Need %d credits. Purchase credits from the store.", cost))
		}

		id := c.Params("id")

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString("Error starting transaction")
		}
		defer tx.Rollback()

		// Deduct credits
		_, err = tx.Exec("UPDATE users SET credits = credits - ? WHERE id = ?", cost, currentUser.ID)
		if err != nil {
			return c.Status(500).SendString("Error updating credits")
		}

		// Boost post (7 days)
		_, err = tx.Exec(`UPDATE posts SET boost_until = datetime('now', '+7 days') WHERE id = ?`, id)
		if err != nil {
			return c.Status(500).SendString("Boost failed")
		}

		// Log transaction
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			currentUser.ID, cost, "debit", "Post boost (7 days)")
		if err != nil {
			log.Println("Error logging transaction:", err)
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
			return c.SendString(fmt.Sprintf("Not enough credits! Need %d credits. Purchase credits from the store.", cost))
		}

		id := c.Params("id")

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString("Error starting transaction")
		}
		defer tx.Rollback()

		// Deduct credits
		_, err = tx.Exec("UPDATE users SET credits = credits - ? WHERE id = ?", cost, currentUser.ID)
		if err != nil {
			return c.Status(500).SendString("Error updating credits")
		}

		// Feature post (24 hours)
		_, err = tx.Exec(`UPDATE posts SET featured_until = datetime('now', '+1 day') WHERE id = ?`, id)
		if err != nil {
			return c.Status(500).SendString("Feature failed")
		}

		// Log transaction
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			currentUser.ID, cost, "debit", "Featured listing (24 hours)")
		if err != nil {
			log.Println("Error logging transaction:", err)
		}

		err = tx.Commit()
		if err != nil {
			return c.Status(500).SendString("Error committing transaction")
		}

		return c.SendString("✅ Post featured for 24 hours! It will appear at the top.")
	})

	// ====================== BUY CREDITS ======================
	app.Post("/buy-credits", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil {
			return c.Status(401).SendString("Please login")
		}

		packageType := c.FormValue("package")
		var credits, amount int

		switch packageType {
		case "small":
			credits, amount = 500, 499
		case "large":
			credits, amount = 1200, 999
		default:
			return c.Status(400).SendString("Invalid package")
		}

		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).SendString("Error starting transaction")
		}
		defer tx.Rollback()

		_, err = tx.Exec("UPDATE users SET credits = credits + ? WHERE id = ?", credits, currentUser.ID)
		if err != nil {
			return c.Status(500).SendString("Error processing payment")
		}

		// Log transaction
		_, err = tx.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			currentUser.ID, amount, "credit", fmt.Sprintf("Purchased %d credits", credits))
		if err != nil {
			log.Println("Error logging transaction:", err)
		}

		err = tx.Commit()
		if err != nil {
			return c.Status(500).SendString("Error committing transaction")
		}

		return c.SendString(fmt.Sprintf(`✅ Success! Added %d credits to your account.<script>window.location.reload()</script>`, credits))
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

		// Get pending posts
		pendingPosts, _ := fetchPosts(`
			SELECT id, username, handle, content, price, category, tags, image_url, likes, created_at,
			       CASE WHEN boost_until > datetime('now') THEN 1 ELSE 0 END,
			       CASE WHEN featured_until > datetime('now') THEN 1 ELSE 0 END,
			       user_id, status
			FROM posts 
			WHERE status = 'pending'
			ORDER BY created_at DESC
		`)

		// Get all users
		rows, err := db.Query(`
			SELECT id, username, handle, email, is_admin, credits, is_premium, membership_tier 
			FROM users ORDER BY id DESC
		`)
		if err != nil {
			log.Println("Error fetching users:", err)
		}
		defer rows.Close()

		var users []User
		for rows.Next() {
			var u User
			err := rows.Scan(&u.ID, &u.Username, &u.Handle, &u.Email, &u.IsAdmin, &u.Credits, &u.IsPremium, &u.MembershipTier)
			if err != nil {
				continue
			}
			users = append(users, u)
		}

		// Get stats
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
		})
	})

	// Approve post
	app.Post("/admin/approve/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		id := c.Params("id")
		_, err := db.Exec("UPDATE posts SET status = 'approved' WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Error approving post")
		}
		return c.SendString("✅ Post approved")
	})

	// Delete post
	app.Post("/admin/delete/:id", func(c *fiber.Ctx) error {
		currentUser := getCurrentUser(c)
		if currentUser == nil || !currentUser.IsAdmin {
			return c.Status(403).SendString("Access denied")
		}

		id := c.Params("id")
		_, err := db.Exec("DELETE FROM posts WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Error deleting post")
		}
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
		
		_, err := db.Exec("UPDATE users SET credits = credits + ? WHERE id = ?", creditsInt, userID)
		if err != nil {
			return c.Status(500).SendString("Error giving credits")
		}
		
		db.Exec("INSERT INTO transactions (user_id, amount, type, description) VALUES (?, ?, ?, ?)",
			userID, creditsInt, "credit", "Admin grant")
		
		return c.SendString(fmt.Sprintf("✅ Added %d credits to user", creditsInt))
	})

	// ====================== AUTH ======================
	app.Post("/login", func(c *fiber.Ctx) error {
		email := c.FormValue("email")
		password := c.FormValue("password")

		var id int
		var username, handle string
		var isAdmin bool
		var hash string

		err := db.QueryRow("SELECT id, username, handle, is_admin, password_hash FROM users WHERE email = ?", email).
			Scan(&id, &username, &handle, &isAdmin, &hash)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
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
		password := c.FormValue("password")

		if len(password) < 6 {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Password must be at least 6 characters</div>`)
		}

		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := db.Exec(`INSERT INTO users (username, handle, email, password_hash, credits) VALUES (?, ?, ?, ?, ?)`,
			username, handle, email, hash, 500)
		if err != nil {
			return c.SendString(`<div class="p-4 text-red-500 text-center">Handle or email already taken</div>`)
		}

		return c.SendString(`<div class="p-4 text-emerald-600 text-center">Account created successfully!<script>window.location.reload()</script></div>`)
	})

	app.Post("/logout", func(c *fiber.Ctx) error {
		c.ClearCookie("auth")
		return c.SendString(`<div class="p-4 text-center">Logged out successfully<script>window.location.reload()</script></div>`)
	})

	log.Println("🚀 Server running on http://localhost:3000")
	log.Fatal(app.Listen(":3000"))
}
