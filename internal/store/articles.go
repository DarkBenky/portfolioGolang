package store

// ArticleFilter narrows the stored knowledge base. Chunks without an owner are shared
// corpus (news, fundamentals); the rest belong to a single user.
type ArticleFilter struct {
	Query  string
	Source string
	Ticker string
	Limit  int
	Offset int
}

type Article struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Ticker    string `json:"ticker"`
	Date      string `json:"date"`
	Content   string `json:"content"`
	HasVector bool   `json:"has_vector"`
	CreatedAt int64  `json:"created_at"`
}

type ArticleSourceCount struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

type ArticleTickerCount struct {
	Ticker string `json:"ticker"`
	Count  int64  `json:"count"`
}

type ArticleStats struct {
	Total          int64                `json:"total"`
	WithEmbeddings int64                `json:"with_embeddings"`
	ContentBytes   int64                `json:"content_bytes"`
	VectorBytes    int64                `json:"vector_bytes"`
	OldestDate     string               `json:"oldest_date"`
	NewestDate     string               `json:"newest_date"`
	Sources        []ArticleSourceCount `json:"sources"`
	Tickers        []ArticleTickerCount `json:"tickers"`
}

const articleScope = `(COALESCE(user_id,'') = '' OR user_id = ?)`

// Articles returns one page of chunks plus the total that matches the filter.
func (d *DB) Articles(userID string, filter ArticleFilter) ([]Article, int, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 40
	}
	where := articleScope
	args := []any{userID}
	if filter.Source != "" {
		where += ` AND source = ?`
		args = append(args, filter.Source)
	}
	if filter.Ticker != "" {
		where += ` AND ticker = ?`
		args = append(args, filter.Ticker)
	}
	if filter.Query != "" {
		where += ` AND instr(lower(content), lower(?)) > 0`
		args = append(args, filter.Query)
	}

	var total int
	if err := d.QueryRow(`SELECT COUNT(*) FROM rag_chunks WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := d.Query(`SELECT id, source, COALESCE(ticker,''), COALESCE(date,''), content,
		length(COALESCE(embedding,'')) > 2 AS has_vector, created_at
		FROM rag_chunks WHERE `+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	articles := make([]Article, 0, filter.Limit)
	for rows.Next() {
		var article Article
		if err := rows.Scan(&article.ID, &article.Source, &article.Ticker, &article.Date,
			&article.Content, &article.HasVector, &article.CreatedAt); err == nil {
			articles = append(articles, article)
		}
	}
	return articles, total, rows.Err()
}

// ArticleStats summarises the knowledge base without loading its content.
func (d *DB) ArticleStats(userID string) (ArticleStats, error) {
	var stats ArticleStats
	err := d.QueryRow(`SELECT COUNT(*), SUM(CASE WHEN length(COALESCE(embedding,'')) > 2 THEN 1 ELSE 0 END),
		COALESCE(SUM(length(content)),0), COALESCE(SUM(length(COALESCE(embedding,''))),0),
		COALESCE(MIN(NULLIF(date,'')),''), COALESCE(MAX(NULLIF(date,'')),'')
		FROM rag_chunks WHERE `+articleScope, userID).
		Scan(&stats.Total, &stats.WithEmbeddings, &stats.ContentBytes, &stats.VectorBytes,
			&stats.OldestDate, &stats.NewestDate)
	if err != nil {
		return stats, err
	}
	stats.Sources = []ArticleSourceCount{}
	if rows, err := d.Query(`SELECT source, COUNT(*) FROM rag_chunks WHERE `+articleScope+
		` GROUP BY 1 ORDER BY 2 DESC`, userID); err == nil {
		for rows.Next() {
			var row ArticleSourceCount
			if rows.Scan(&row.Source, &row.Count) == nil {
				stats.Sources = append(stats.Sources, row)
			}
		}
		rows.Close()
	}
	stats.Tickers = []ArticleTickerCount{}
	if rows, err := d.Query(`SELECT ticker, COUNT(*) FROM rag_chunks WHERE `+articleScope+
		` AND COALESCE(ticker,'') <> '' GROUP BY 1 ORDER BY 2 DESC LIMIT 15`, userID); err == nil {
		for rows.Next() {
			var row ArticleTickerCount
			if rows.Scan(&row.Ticker, &row.Count) == nil {
				stats.Tickers = append(stats.Tickers, row)
			}
		}
		rows.Close()
	}
	return stats, nil
}
