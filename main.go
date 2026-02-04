package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const (
	githubAPIBase = "https://api.github.com"
)

type Config struct {
	Token   string
	Owner   string
	Repos   []string
	Members []string
	From    time.Time
	To      time.Time
}

type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type Review struct {
	ID          int       `json:"id"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submitted_at"`
}

type Commit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type PRCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type Metrics struct {
	Repo                    string
	Period                  string
	Days                    int
	DeploymentFrequency     float64
	DeploymentsTotal        int
	LeadTimeForChanges      time.Duration
	LeadTimeMedian          time.Duration
	ChangeFailureRate       float64
	FailureCount            int
	TimeToFirstReview       time.Duration
	TimeToFirstReviewMedian time.Duration
	PRsAnalyzed             int
	MemberMetrics           map[string]*MemberMetrics
}

type MemberMetrics struct {
	Username            string
	PRsMerged           int
	AvgLeadTime         time.Duration
	MedianLeadTime      time.Duration
	AvgTimeToFirstReview time.Duration
	MedianTimeToFirstReview time.Duration
	FailurePRs          int
	LeadTimes           []time.Duration
	FirstReviewTimes    []time.Duration
}

type Client struct {
	httpClient *http.Client
	token      string
}

func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
	}
}

func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API error: %s (status %d)", url, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) fetchAllPages(ctx context.Context, baseURL string, perPage int, result interface{}) error {
	// This is a simplified version - for production, implement proper pagination
	url := fmt.Sprintf("%s?per_page=%d&state=all", baseURL, perPage)
	return c.doRequest(ctx, url, result)
}

func (c *Client) GetMergedPRs(ctx context.Context, owner, repo string, since, until time.Time, members []string) ([]PullRequest, error) {
	memberSet := make(map[string]bool)
	for _, m := range members {
		memberSet[strings.ToLower(m)] = true
	}

	var allPRs []PullRequest
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=closed&sort=updated&direction=desc&per_page=%d&page=%d",
			githubAPIBase, owner, repo, perPage, page)

		var prs []PullRequest
		if err := c.doRequest(ctx, url, &prs); err != nil {
			return nil, err
		}

		if len(prs) == 0 {
			break
		}

		foundOld := false
		for _, pr := range prs {
			if pr.MergedAt == nil {
				continue
			}
			if pr.MergedAt.Before(since) {
				foundOld = true
				continue
			}
			if pr.MergedAt.After(until) {
				continue
			}
			// Filter by members if specified
			if len(members) > 0 && !memberSet[strings.ToLower(pr.User.Login)] {
				continue
			}
			allPRs = append(allPRs, pr)
		}

		if foundOld || len(prs) < perPage {
			break
		}
		page++
	}

	return allPRs, nil
}

func (c *Client) GetPRCommits(ctx context.Context, owner, repo string, prNumber int) ([]PRCommit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/commits?per_page=100",
		githubAPIBase, owner, repo, prNumber)

	var commits []PRCommit
	if err := c.doRequest(ctx, url, &commits); err != nil {
		return nil, err
	}
	return commits, nil
}

func (c *Client) GetPRReviews(ctx context.Context, owner, repo string, prNumber int) ([]Review, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/reviews",
		githubAPIBase, owner, repo, prNumber)

	var reviews []Review
	if err := c.doRequest(ctx, url, &reviews); err != nil {
		return nil, err
	}
	return reviews, nil
}

func (c *Client) GetRecentCommits(ctx context.Context, owner, repo, branch string, since time.Time) ([]Commit, error) {
	var allCommits []Commit
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/commits?sha=%s&since=%s&per_page=%d&page=%d",
			githubAPIBase, owner, repo, branch, since.Format(time.RFC3339), perPage, page)

		var commits []Commit
		if err := c.doRequest(ctx, url, &commits); err != nil {
			return nil, err
		}

		if len(commits) == 0 {
			break
		}

		allCommits = append(allCommits, commits...)

		if len(commits) < perPage {
			break
		}
		page++
	}

	return allCommits, nil
}

func isFailurePR(pr PullRequest) bool {
	// Check branch name
	branchLower := strings.ToLower(pr.Head.Ref)
	if strings.HasPrefix(branchLower, "hotfix") || strings.HasPrefix(branchLower, "bugfix") ||
		strings.Contains(branchLower, "hotfix") || strings.Contains(branchLower, "bugfix") {
		return true
	}

	// Check labels
	for _, label := range pr.Labels {
		labelLower := strings.ToLower(label.Name)
		if labelLower == "bug" || labelLower == "hotfix" || labelLower == "bugfix" ||
			strings.Contains(labelLower, "bug") || strings.Contains(labelLower, "hotfix") {
			return true
		}
	}

	return false
}

func isRevertCommit(commit Commit) bool {
	msg := strings.ToLower(commit.Commit.Message)
	return strings.HasPrefix(msg, "revert")
}

func median(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func average(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

func calculateMetrics(ctx context.Context, client *Client, cfg Config, repo string) (*Metrics, error) {
	since := cfg.From
	until := cfg.To

	fmt.Printf("\n📊 Analyzing %s/%s...\n", cfg.Owner, repo)

	// Get merged PRs
	prs, err := client.GetMergedPRs(ctx, cfg.Owner, repo, since, until, cfg.Members)
	if err != nil {
		return nil, fmt.Errorf("failed to get PRs: %w", err)
	}
	fmt.Printf("   Found %d merged PRs\n", len(prs))

	// Get commits on main for revert detection
	commits, err := client.GetRecentCommits(ctx, cfg.Owner, repo, "main", since)
	if err != nil {
		// Try master if main doesn't exist
		commits, err = client.GetRecentCommits(ctx, cfg.Owner, repo, "master", since)
		if err != nil {
			fmt.Printf("   Warning: Could not fetch commits: %v\n", err)
			commits = []Commit{}
		}
	}

	// Count reverts
	revertCount := 0
	for _, c := range commits {
		if isRevertCommit(c) {
			revertCount++
		}
	}

	periodDays := int(until.Sub(since).Hours()/24) + 1
	metrics := &Metrics{
		Repo:          repo,
		Period:        fmt.Sprintf("%s ~ %s", since.Format("2006-01-02"), until.Format("2006-01-02")),
		Days:          periodDays,
		MemberMetrics: make(map[string]*MemberMetrics),
	}

	// Initialize member metrics
	for _, member := range cfg.Members {
		metrics.MemberMetrics[strings.ToLower(member)] = &MemberMetrics{
			Username: member,
		}
	}

	var leadTimes []time.Duration
	var firstReviewTimes []time.Duration
	failureCount := 0

	for i, pr := range prs {
		if (i+1)%10 == 0 {
			fmt.Printf("   Processing PR %d/%d...\n", i+1, len(prs))
		}

		memberKey := strings.ToLower(pr.User.Login)
		memberMetric, ok := metrics.MemberMetrics[memberKey]
		if !ok {
			memberMetric = &MemberMetrics{Username: pr.User.Login}
			metrics.MemberMetrics[memberKey] = memberMetric
		}
		memberMetric.PRsMerged++

		// Calculate lead time (first commit to merge)
		prCommits, err := client.GetPRCommits(ctx, cfg.Owner, repo, pr.Number)
		if err != nil {
			fmt.Printf("   Warning: Could not get commits for PR #%d: %v\n", pr.Number, err)
			continue
		}

		if len(prCommits) > 0 && pr.MergedAt != nil {
			firstCommitTime := prCommits[0].Commit.Author.Date
			for _, c := range prCommits {
				if c.Commit.Author.Date.Before(firstCommitTime) {
					firstCommitTime = c.Commit.Author.Date
				}
			}
			leadTime := pr.MergedAt.Sub(firstCommitTime)
			leadTimes = append(leadTimes, leadTime)
			memberMetric.LeadTimes = append(memberMetric.LeadTimes, leadTime)
		}

		// Calculate time to first review
		reviews, err := client.GetPRReviews(ctx, cfg.Owner, repo, pr.Number)
		if err != nil {
			fmt.Printf("   Warning: Could not get reviews for PR #%d: %v\n", pr.Number, err)
		} else if len(reviews) > 0 {
			// Find first review (excluding author's own reviews)
			var firstReviewTime *time.Time
			for _, review := range reviews {
				if strings.ToLower(review.User.Login) == strings.ToLower(pr.User.Login) {
					continue
				}
				if firstReviewTime == nil || review.SubmittedAt.Before(*firstReviewTime) {
					t := review.SubmittedAt
					firstReviewTime = &t
				}
			}
			if firstReviewTime != nil {
				timeToFirstReview := firstReviewTime.Sub(pr.CreatedAt)
				firstReviewTimes = append(firstReviewTimes, timeToFirstReview)
				memberMetric.FirstReviewTimes = append(memberMetric.FirstReviewTimes, timeToFirstReview)
			}
		}

		// Check for failure
		if isFailurePR(pr) {
			failureCount++
			memberMetric.FailurePRs++
		}
	}

	// Calculate aggregate metrics
	days := int(cfg.To.Sub(cfg.From).Hours()/24) + 1
	metrics.DeploymentsTotal = len(prs)
	metrics.DeploymentFrequency = float64(len(prs)) / float64(days)
	metrics.LeadTimeForChanges = average(leadTimes)
	metrics.LeadTimeMedian = median(leadTimes)
	metrics.FailureCount = failureCount + revertCount
	if len(prs) > 0 {
		metrics.ChangeFailureRate = float64(failureCount+revertCount) / float64(len(prs)) * 100
	}
	metrics.TimeToFirstReview = average(firstReviewTimes)
	metrics.TimeToFirstReviewMedian = median(firstReviewTimes)
	metrics.PRsAnalyzed = len(prs)

	// Calculate member-level metrics
	for _, mm := range metrics.MemberMetrics {
		mm.AvgLeadTime = average(mm.LeadTimes)
		mm.MedianLeadTime = median(mm.LeadTimes)
		mm.AvgTimeToFirstReview = average(mm.FirstReviewTimes)
		mm.MedianTimeToFirstReview = median(mm.FirstReviewTimes)
	}

	return metrics, nil
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "N/A"
	}
	hours := d.Hours()
	if hours < 24 {
		return fmt.Sprintf("%.1f hours", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%.1f days", days)
}

func printMetrics(metrics *Metrics) {
	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  DORA Metrics: %s\n", metrics.Repo)
	fmt.Printf("║  Period: %s\n", metrics.Period)
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║\n")
	fmt.Printf("║  📦 Deployment Frequency\n")
	fmt.Printf("║     Total Deployments: %d\n", metrics.DeploymentsTotal)
	fmt.Printf("║     Frequency: %.2f deploys/day (%.1f deploys/week)\n",
		metrics.DeploymentFrequency, metrics.DeploymentFrequency*7)
	fmt.Printf("║\n")
	fmt.Printf("║  ⏱️  Lead Time for Changes (first commit → merge)\n")
	fmt.Printf("║     Average: %s\n", formatDuration(metrics.LeadTimeForChanges))
	fmt.Printf("║     Median:  %s\n", formatDuration(metrics.LeadTimeMedian))
	fmt.Printf("║\n")
	fmt.Printf("║  🔥 Change Failure Rate\n")
	fmt.Printf("║     Failure PRs + Reverts: %d\n", metrics.FailureCount)
	fmt.Printf("║     Rate: %.1f%%\n", metrics.ChangeFailureRate)
	fmt.Printf("║\n")
	fmt.Printf("║  👀 Time to First Review (PR created → first review)\n")
	fmt.Printf("║     Average: %s\n", formatDuration(metrics.TimeToFirstReview))
	fmt.Printf("║     Median:  %s\n", formatDuration(metrics.TimeToFirstReviewMedian))
	fmt.Printf("║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  👥 Per-Member Breakdown\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")

	for _, mm := range metrics.MemberMetrics {
		if mm.PRsMerged == 0 {
			continue
		}
		fmt.Printf("║\n")
		fmt.Printf("║  @%s\n", mm.Username)
		fmt.Printf("║     PRs Merged: %d\n", mm.PRsMerged)
		fmt.Printf("║     Lead Time (avg/median): %s / %s\n",
			formatDuration(mm.AvgLeadTime), formatDuration(mm.MedianLeadTime))
		fmt.Printf("║     Time to First Review (avg/median): %s / %s\n",
			formatDuration(mm.AvgTimeToFirstReview), formatDuration(mm.MedianTimeToFirstReview))
		fmt.Printf("║     Failure PRs: %d\n", mm.FailurePRs)
	}

	fmt.Printf("║\n")
	fmt.Printf("╚══════════════════════════════════════════════════════════════════╝\n")
}

func printSummary(allMetrics []*Metrics) {
	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  📊 COMBINED SUMMARY (All Repositories)\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")

	var totalDeploys int
	var totalFailures int
	var allLeadTimes []time.Duration
	var allFirstReviewTimes []time.Duration
	combinedMembers := make(map[string]*MemberMetrics)
	days := 0
	if len(allMetrics) > 0 {
		days = allMetrics[0].Days
	}

	for _, m := range allMetrics {
		totalDeploys += m.DeploymentsTotal
		totalFailures += m.FailureCount

		for key, mm := range m.MemberMetrics {
			if _, ok := combinedMembers[key]; !ok {
				combinedMembers[key] = &MemberMetrics{Username: mm.Username}
			}
			combinedMembers[key].PRsMerged += mm.PRsMerged
			combinedMembers[key].FailurePRs += mm.FailurePRs
			combinedMembers[key].LeadTimes = append(combinedMembers[key].LeadTimes, mm.LeadTimes...)
			combinedMembers[key].FirstReviewTimes = append(combinedMembers[key].FirstReviewTimes, mm.FirstReviewTimes...)
			allLeadTimes = append(allLeadTimes, mm.LeadTimes...)
			allFirstReviewTimes = append(allFirstReviewTimes, mm.FirstReviewTimes...)
		}
	}

	// Calculate combined member metrics
	for _, mm := range combinedMembers {
		mm.AvgLeadTime = average(mm.LeadTimes)
		mm.MedianLeadTime = median(mm.LeadTimes)
		mm.AvgTimeToFirstReview = average(mm.FirstReviewTimes)
		mm.MedianTimeToFirstReview = median(mm.FirstReviewTimes)
	}

	fmt.Printf("║\n")
	fmt.Printf("║  📦 Deployment Frequency (All Repos)\n")
	fmt.Printf("║     Total Deployments: %d\n", totalDeploys)
	fmt.Printf("║     Frequency: %.2f deploys/day (%.1f deploys/week)\n",
		float64(totalDeploys)/float64(days), float64(totalDeploys)/float64(days)*7)
	fmt.Printf("║\n")
	fmt.Printf("║  ⏱️  Lead Time for Changes (All Repos)\n")
	fmt.Printf("║     Average: %s\n", formatDuration(average(allLeadTimes)))
	fmt.Printf("║     Median:  %s\n", formatDuration(median(allLeadTimes)))
	fmt.Printf("║\n")
	fmt.Printf("║  🔥 Change Failure Rate (All Repos)\n")
	fmt.Printf("║     Failures: %d / %d\n", totalFailures, totalDeploys)
	if totalDeploys > 0 {
		fmt.Printf("║     Rate: %.1f%%\n", float64(totalFailures)/float64(totalDeploys)*100)
	}
	fmt.Printf("║\n")
	fmt.Printf("║  👀 Time to First Review (All Repos)\n")
	fmt.Printf("║     Average: %s\n", formatDuration(average(allFirstReviewTimes)))
	fmt.Printf("║     Median:  %s\n", formatDuration(median(allFirstReviewTimes)))
	fmt.Printf("║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║  👥 Combined Per-Member Metrics\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════════╣\n")

	for _, mm := range combinedMembers {
		if mm.PRsMerged == 0 {
			continue
		}
		fmt.Printf("║\n")
		fmt.Printf("║  @%s\n", mm.Username)
		fmt.Printf("║     PRs Merged: %d\n", mm.PRsMerged)
		fmt.Printf("║     Lead Time (avg/median): %s / %s\n",
			formatDuration(mm.AvgLeadTime), formatDuration(mm.MedianLeadTime))
		fmt.Printf("║     Time to First Review (avg/median): %s / %s\n",
			formatDuration(mm.AvgTimeToFirstReview), formatDuration(mm.MedianTimeToFirstReview))
		fmt.Printf("║     Failure PRs: %d\n", mm.FailurePRs)
	}

	fmt.Printf("║\n")
	fmt.Printf("╚══════════════════════════════════════════════════════════════════╝\n")
}

func main() {
	// Load .env file (optional - won't error if not found)
	_ = godotenv.Load()

	// Define flags with defaults from environment variables
	owner := flag.String("owner", os.Getenv("GITHUB_OWNER"), "GitHub organization or user (required)")
	repos := flag.String("repos", os.Getenv("GITHUB_REPOS"), "Comma-separated list of repository names (required)")
	members := flag.String("members", os.Getenv("GITHUB_MEMBERS"), "Comma-separated list of GitHub usernames to filter (optional)")
	fromStr := flag.String("from", os.Getenv("DORA_FROM"), "Start date (YYYY-MM-DD, required)")
	toStr := flag.String("to", os.Getenv("DORA_TO"), "End date (YYYY-MM-DD, required)")
	token := flag.String("token", "", "GitHub API token (or set GITHUB_TOKEN env var)")

	flag.Parse()

	// Validate required flags
	if *owner == "" {
		fmt.Println("Error: --owner is required")
		flag.Usage()
		os.Exit(1)
	}
	if *repos == "" {
		fmt.Println("Error: --repos is required")
		flag.Usage()
		os.Exit(1)
	}
	if *fromStr == "" {
		fmt.Println("Error: --from is required (format: YYYY-MM-DD)")
		flag.Usage()
		os.Exit(1)
	}
	if *toStr == "" {
		fmt.Println("Error: --to is required (format: YYYY-MM-DD)")
		flag.Usage()
		os.Exit(1)
	}

	// Parse dates
	fromDate, err := time.Parse("2006-01-02", *fromStr)
	if err != nil {
		fmt.Printf("Error: Invalid --from date format: %v\n", err)
		os.Exit(1)
	}
	toDate, err := time.Parse("2006-01-02", *toStr)
	if err != nil {
		fmt.Printf("Error: Invalid --to date format: %v\n", err)
		os.Exit(1)
	}
	// Set toDate to end of day
	toDate = toDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	if fromDate.After(toDate) {
		fmt.Println("Error: --from date must be before --to date")
		os.Exit(1)
	}

	// Get token from flag or env
	apiToken := *token
	if apiToken == "" {
		apiToken = os.Getenv("GITHUB_TOKEN")
	}
	if apiToken == "" {
		fmt.Println("Error: GitHub token required. Use --token flag or set GITHUB_TOKEN env var")
		os.Exit(1)
	}

	// Parse repos and members
	repoList := strings.Split(*repos, ",")
	for i := range repoList {
		repoList[i] = strings.TrimSpace(repoList[i])
	}

	var memberList []string
	if *members != "" {
		memberList = strings.Split(*members, ",")
		for i := range memberList {
			memberList[i] = strings.TrimSpace(memberList[i])
		}
	}

	cfg := Config{
		Token:   apiToken,
		Owner:   *owner,
		Repos:   repoList,
		Members: memberList,
		From:    fromDate,
		To:      toDate,
	}

	fmt.Printf("🚀 DORA Metrics Calculator\n")
	fmt.Printf("   Organization: %s\n", cfg.Owner)
	fmt.Printf("   Repositories: %v\n", cfg.Repos)
	if len(cfg.Members) > 0 {
		fmt.Printf("   Members: %v\n", cfg.Members)
	} else {
		fmt.Printf("   Members: All contributors\n")
	}
	fmt.Printf("   Period: %s ~ %s\n", cfg.From.Format("2006-01-02"), cfg.To.Format("2006-01-02"))

	ctx := context.Background()
	client := NewClient(cfg.Token)

	var allMetrics []*Metrics

	for _, repo := range cfg.Repos {
		metrics, err := calculateMetrics(ctx, client, cfg, repo)
		if err != nil {
			fmt.Printf("Error analyzing %s: %v\n", repo, err)
			continue
		}
		allMetrics = append(allMetrics, metrics)
		printMetrics(metrics)
	}

	// Print combined summary if multiple repos
	if len(allMetrics) > 1 {
		printSummary(allMetrics)
	}

	fmt.Printf("\n✅ Analysis complete!\n")
}
