package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

type Stats struct {
	TotalPRs       int
	TotalLeadTime  time.Duration
	BugFixPRs      int // "不具合修正/パッチ対応" を行った数
	FeaturePRs     int // "新規・機能改善" を行った数
	TotalAdditions int
}

func main() {
	_ = godotenv.Load()

	ownerFlag := flag.String("owner", os.Getenv("TARGET_OWNER"), "GitHub Owner/Org name")
	reposFlag := flag.String("repos", os.Getenv("TARGET_REPOS"), "Comma-separated repository names")
	membersFlag := flag.String("members", os.Getenv("TARGET_MEMBERS"), "Comma-separated GitHub usernames to filter")
	startFlag := flag.String("start", os.Getenv("DORA_FROM"), "Start date (YYYY-MM-DD)")
	endFlag := flag.String("end", os.Getenv("DORA_TO"), "End date (YYYY-MM-DD)")
	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || *ownerFlag == "" || *reposFlag == "" || *startFlag == "" || *endFlag == "" {
		log.Fatal("❌ Error: Missing required parameters.")
	}

	repos := strings.Split(*reposFlag, ",")
	memberMap := make(map[string]bool)
	if *membersFlag != "" {
		for _, m := range strings.Split(*membersFlag, ",") {
			memberMap[strings.TrimSpace(m)] = true
		}
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	teamStats := &Stats{}
	repoStatsMap := make(map[string]*Stats)
	userStatsMap := make(map[string]*Stats)
	var mu sync.Mutex

	fmt.Printf("🚀 Analyzing: %s to %s\n", *startFlag, *endFlag)

	for _, repoName := range repos {
		repoName = strings.TrimSpace(repoName)
		repoStats := &Stats{}
		query := fmt.Sprintf("repo:%s/%s is:pr is:merged merged:%s..%s", *ownerFlag, repoName, *startFlag, *endFlag)
		allIssues := fetchAllIssues(ctx, client, query)

		if len(allIssues) == 0 {
			repoStatsMap[repoName] = repoStats
			continue
		}

		prChan := make(chan int, len(allIssues))
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for num := range prChan {
					pr, _, err := client.PullRequests.Get(ctx, *ownerFlag, repoName, num)
					if err != nil { continue }

					author := pr.GetUser().GetLogin()
					if len(memberMap) > 0 && !memberMap[author] { continue }

					// Bug判定（タイトル、ラベル、ブランチ、セキュリティパッチ含む）
					isFix := isBugFix(pr)
					lt := pr.GetMergedAt().Sub(pr.GetCreatedAt().Time)

					mu.Lock()
					if userStatsMap[author] == nil { userStatsMap[author] = &Stats{} }
					update(teamStats, lt, isFix, pr.GetAdditions())
					update(repoStats, lt, isFix, pr.GetAdditions())
					update(userStatsMap[author], lt, isFix, pr.GetAdditions())
					mu.Unlock()
				}
			}()
		}
		for _, issue := range allIssues { prChan <- issue.GetNumber() }
		close(prChan)
		wg.Wait()
		repoStatsMap[repoName] = repoStats
	}

	displayResults(*startFlag, *endFlag, teamStats, repoStatsMap, userStatsMap)
}

func isBugFix(pr *github.PullRequest) bool {
	title := strings.ToLower(pr.GetTitle())
	branch := strings.ToLower(pr.GetHead().GetRef())
	keywords := []string{"bug", "fix", "hotfix", "defect", "incident", "patch", "security", "dependabot", "不具合", "修正"}

	for _, l := range pr.Labels {
		ln := strings.ToLower(l.GetName())
		for _, k := range keywords {
			if strings.Contains(ln, k) { return true }
		}
	}
	for _, k := range keywords {
		if strings.Contains(title, k) || strings.Contains(branch, k) { return true }
	}
	return false
}

func update(s *Stats, lt time.Duration, isFix bool, add int) {
	s.TotalPRs++
	s.TotalLeadTime += lt
	s.TotalAdditions += add
	if isFix {
		s.BugFixPRs++
	} else {
		s.FeaturePRs++
	}
}

func displayResults(from, to string, team *Stats, repos map[string]*Stats, users map[string]*Stats) {
	line := strings.Repeat("-", 100)
	fmt.Printf("\n%s\n📊 DORA & Contribution Summary (%s - %s)\n%s\n", line, from, to, line)

	// チーム全体のDORA
	fmt.Printf("%-25s | %-8s | %-10s | %-10s | %-10s\n", "ENTITY", "PRs", "AvgLT", "CFR", "AvgSize")
	printRow("OVERALL TEAM", team, true)
	fmt.Println(line)

	// リポジトリ別
	for name, s := range repos {
		printRow(name, s, true)
	}
	fmt.Println(line)

	// 個人別（見せ方を変える）
	fmt.Printf("%-25s | %-8s | %-10s | %-15s | %-10s\n", "CONTRIBUTOR", "TotalPRs", "NewWork", "Fix/Maintenance", "AvgSize")
	for user, s := range users {
		newWork := s.FeaturePRs
		fixes := s.BugFixPRs
		avgSize := 0
		if s.TotalPRs > 0 { avgSize = s.TotalAdditions / s.TotalPRs }

		fmt.Printf("%-25s | %8d | %10d | %15d | +%d lines\n",
			user, s.TotalPRs, newWork, fixes, avgSize)
	}
}

func printRow(name string, s *Stats, showCFR bool) {
	avgLT, cfr, avgAdd := 0.0, 0.0, 0
	if s.TotalPRs > 0 {
		avgLT = s.TotalLeadTime.Hours() / float64(s.TotalPRs)
		cfr = float64(s.BugFixPRs) / float64(s.TotalPRs) * 100
		avgAdd = s.TotalAdditions / s.TotalPRs
	}
	fmt.Printf("%-25s | %8d | %8.1fh | %8.1f%% | +%d\n",
		name, s.TotalPRs, avgLT, cfr, avgAdd)
}

func fetchAllIssues(ctx context.Context, client *github.Client, query string) []*github.Issue {
	var allIssues []*github.Issue
	opts := &github.SearchOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		result, resp, err := client.Search.Issues(ctx, query, opts)
		if err != nil { return nil }
		allIssues = append(allIssues, result.Issues...)
		if resp.NextPage == 0 { break }
		opts.Page = resp.NextPage
	}
	return allIssues
}