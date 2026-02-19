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
	BugFixPRs      int
	TotalAdditions int
	TotalDeletions int
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
		log.Fatal("❌ Error: Missing required parameters in .env or flags.")
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

					// --- 強化されたBug判定ロジック ---
					isBug := false
					title := strings.ToLower(pr.GetTitle())
					branch := strings.ToLower(pr.GetHead().GetRef())

					// 判定キーワード
					keywords := []string{"bug", "fix", "hotfix", "defect", "incident", "不具合", "修正"}

					// 1. ラベルチェック
					for _, l := range pr.Labels {
						labelName := strings.ToLower(l.GetName())
						for _, k := range keywords {
							if strings.Contains(labelName, k) { isBug = true; break }
						}
					}
					// 2. タイトルチェック (ラベルで見つからなかった場合)
					if !isBug {
						for _, k := range keywords {
							if strings.Contains(title, k) { isBug = true; break }
						}
					}
					// 3. ブランチ名チェック
					if !isBug {
						for _, k := range keywords {
							if strings.Contains(branch, k) { isBug = true; break }
						}
					}

					// デバッグ出力: バグと判定されたPRを表示
					if isBug {
						fmt.Printf("  [BUG Detect] #%d: %s (Author: %s)\n", pr.GetNumber(), pr.GetTitle(), author)
					}

					lt := pr.GetMergedAt().Sub(pr.GetCreatedAt().Time)
					mu.Lock()
					if userStatsMap[author] == nil { userStatsMap[author] = &Stats{} }
					update(teamStats, lt, isBug, pr.GetAdditions(), pr.GetDeletions())
					update(repoStats, lt, isBug, pr.GetAdditions(), pr.GetDeletions())
					update(userStatsMap[author], lt, isBug, pr.GetAdditions(), pr.GetDeletions())
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

// --- 以下、補助関数 ---
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

func update(s *Stats, lt time.Duration, isBug bool, add, del int) {
	s.TotalPRs++
	s.TotalLeadTime += lt
	s.TotalAdditions += add
	s.TotalDeletions += del
	if isBug { s.BugFixPRs++ }
}

func displayResults(from, to string, team *Stats, repos map[string]*Stats, users map[string]*Stats) {
	line := strings.Repeat("=", 85)
	fmt.Printf("\n%s\n📊 DORA Metrics Summary (%s - %s)\n%s\n", line, from, to, line)

	fmt.Println("[OVERALL TEAM]")
	printRow("TOTAL", team)

	fmt.Println("\n[BY REPOSITORY]")
	for name, s := range repos {
		printRow(name, s)
	}

	fmt.Println("\n[BY CONTRIBUTOR]")
	for user, s := range users {
		printRow(user, s)
	}
}

func printRow(name string, s *Stats) {
	avgLT, cfr, avgAdd := 0.0, 0.0, 0
	if s.TotalPRs > 0 {
		avgLT = s.TotalLeadTime.Hours() / float64(s.TotalPRs)
		cfr = float64(s.BugFixPRs) / float64(s.TotalPRs) * 100
		avgAdd = s.TotalAdditions / s.TotalPRs
	}
	fmt.Printf("%-25s | PRs: %3d | AvgLT: %5.1fh | CFR: %5.1f%% | AvgSize: +%d lines\n",
		name, s.TotalPRs, avgLT, cfr, avgAdd)
}