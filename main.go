package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/gorilla/feeds"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday"
)

var (
	id = flag.String("id", "", "gist id")
)

var linkRegex = regexp.MustCompile(`<(.*)>; rel="next"`)

type Gist struct {
	Description string `json:"description"`
	Owner       struct {
		Login string `json:"login"`
	} `json:"owner"`
	UpdatedAt string `json:"updated_at"`
}

type Comment struct {
	Id   int    `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	UpdatedAt string `json:"updated_at"`
}

func main() {
	flag.Parse()
	if err := _main(); err != nil {
		log.Fatal(err)
	}
}

func _main() error {
	gist, err := fetchGist(*id)
	if err != nil {
		return err
	}
	comments, err := fetchComments(*id)
	if err != nil {
		return err
	}
	feed, err := buildFeed(*id, gist, comments)
	if err != nil {
		return err
	}
	rss, err := feed.ToRss()
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, rss)
	return nil
}

func fetchComments(id string) ([]Comment, error) {
	var comments []Comment
	var _fetchComments func(url string) error
	_fetchComments = func(url string) error {
		res, err := http.Get(url)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != 200 {
			return fmt.Errorf("failed to acess code:%d", res.StatusCode)
		}
		var _comments []Comment
		if err := json.NewDecoder(res.Body).Decode(&_comments); err != nil {
			return err
		}
		comments = append(comments, _comments...)

		matches := linkRegex.FindStringSubmatch(res.Header.Get("Link"))
		if len(matches) == 2 {
			return _fetchComments(matches[1])
		} else {
			return nil
		}
	}

	if err := _fetchComments(fmt.Sprintf("https://api.github.com/gists/%s/comments", id)); err != nil {
		return nil, err
	}

	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(comments)/2 - 1; i >= 0; i-- {
		opp := len(comments) - 1 - i
		comments[i], comments[opp] = comments[opp], comments[i]
	}

	return comments, nil
}

func fetchGist(id string) (*Gist, error) {
	res, err := http.Get(fmt.Sprintf("https://api.github.com/gists/%s", id))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var gist Gist
	if err := json.NewDecoder(res.Body).Decode(&gist); err != nil {
		return nil, err
	}
	return &gist, nil
}

func buildFeed(id string, gist *Gist, comments []Comment) (*feeds.Feed, error) {
	t, err := time.Parse(time.RFC3339, gist.UpdatedAt)
	if err != nil {
		return nil, err
	}
	feed := &feeds.Feed{
		Title:   gist.Description,
		Link:    &feeds.Link{Href: fmt.Sprintf("https://gist.github.com/%s", id)},
		Author:  &feeds.Author{Name: gist.Owner.Login},
		Created: t,
	}
	for _, comment := range comments {
		t, err := time.Parse(time.RFC3339, comment.UpdatedAt)
		if err != nil {
			return nil, err
		}
		unsafe := blackfriday.MarkdownCommon([]byte(comment.Body))
		html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
		feed.Items = append(feed.Items, &feeds.Item{
			Title:       fmt.Sprintf("%s commented on %s", comment.User.Login, t.Format("02 Jan 2006")),
			Link:        &feeds.Link{Href: fmt.Sprintf("https://gist.github.com/%s#gistcomment-%d", id, comment.Id)},
			Description: string(html),
			Author:      &feeds.Author{Name: comment.User.Login},
			Created:     t,
		})
	}
	return feed, nil
}
