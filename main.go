package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "math/rand"
    "net/http"
    "os"
    "regexp"
    "strconv"
    "strings"
)

type Request struct {
    Token string `json:"token"`
    Type  string `json:"type"`

    // challenge request fields
    Challenge string `json:"challenge"`

    // events
    Event Event `json:"event"`
}

type Event struct {
    Type    string `json:"type"`
    Text    string `json:"text"`
    Channel string `json:"channel"`
}
type VerificationResponse struct {
    Challenge string `json:"challenge"`
}

type PostMessage struct {
    Token   string `json:"token"`
    Channel string `json:"channel"`
    Text    string `json:"text"`
}

func handle(w http.ResponseWriter, r *http.Request) {
    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        fmt.Printf("error: %v\n", err)
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    req := Request{}
    if err := json.Unmarshal([]byte(body), &req); err != nil {
        fmt.Printf("error: %v\n", err)
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    //fmt.Printf("struct: %#v", req)
    if req.Type == "url_verification" {
        resp := VerificationResponse{Challenge: req.Challenge}
        w.Header().Set("Content-type", "application/json")
        w.WriteHeader(http.StatusOK)
        respJson, _ := json.Marshal(resp)
        io.WriteString(w, string(respJson))
        return
    }

    if req.Type == "event_callback" {
        fmt.Printf("saw message text: %s\n", req.Event.Text)

        msg := PostMessage{}
        //msg.Token = auth_token
        msg.Channel = req.Event.Channel
        location, err := PickLocation(req.Event.Text)

        msg.Text = fmt.Sprintf("I recommend %s", location)
        if err != nil {
            msg.Text = fmt.Sprintf("Sorry, I couldn't process that request: %v", err)
        }
        // never output our own name, so we don't trigger ourselves
        fmt.Printf("original: %s\n", msg.Text)
        msg.Text = strings.Replace(msg.Text, "<@UE23Q9BFY>", "lunchbot", -1)
        fmt.Printf("replaced: %s\n", msg.Text)

        msgJson, _ := json.Marshal(msg)

        fmt.Printf("msg json: %s\n", msgJson)
        req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(msgJson))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", auth_token))

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil {
            fmt.Printf("error posting chat message: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
        }
        fmt.Printf("chat message response: %#v\n", resp)
        resp.Body.Close()

        w.WriteHeader(http.StatusOK)

        //respJson, _ := json.Marshal(resp)
        //io.WriteString(w, string(respJson))
    }
}

func PickLocation(text string) (string, error) {
    tags, groupSize, err := Parse(text)
    if err != nil {
        return "", err
    }
    filteredLocations := FilterLocations(tags, groupSize)
    if len(filteredLocations) == 0 {
        return "", fmt.Errorf("no locations matched the specified requirements")
    }
    i := rand.Intn(len(filteredLocations))
    return filteredLocations[i].Name, nil
}

func FilterLocations(tags []string, size int) []Location {
    fmt.Printf("filtering by tags: %q, size: %d\n", tags, size)
    candidates := []Location{}
OUTER:
    for _, l := range Locations {
        if size > 0 && l.Capacity < size {
            continue
        }
        if len(tags) > 0 {
            for _, t := range tags {
                if _, ok := l.Tags[t]; !ok {
                    fmt.Printf("%s failed on tag %s\n", l.Name, t)
                    continue OUTER
                }
                fmt.Printf("%s has tag %s\n", l.Name, t)
            }
        }
        candidates = append(candidates, l)
        fmt.Printf("candidates is now %q\n", candidates)
    }
    return candidates
}
func Parse(text string) ([]string, int, error) {
    matches := parser.FindStringSubmatch(text)
    if len(matches) == 0 {
        return []string{}, -1, fmt.Errorf("could not parse request text: %s", text)
    }
    fmt.Printf("parsed: %q\n", matches)
    groupSize := 0
    g := matches[len(matches)-1]
    if len(g) == 0 {
        groupSize = -1
    } else {
        var err error
        groupSize, err = strconv.Atoi(g)
        if err != nil {
            return []string{}, -1, fmt.Errorf("could not parse group size")
        }
    }
    tags := []string{}
    if len(matches[1]) > 0 {
        tags = strings.Split(matches[1], ",")
        for i, _ := range tags {
            tags[i] = strings.TrimSpace(tags[i])
        }
    }

    return tags, groupSize, nil
}

// msg format:  tag1, tag2, tag3 lunch for 6 people
var parser = regexp.MustCompile(`<@UE23Q9BFY> (.*?)lunch.*?(\d*?)$`)
var auth_token string

func main() {
    auth_token = os.Getenv("TOKEN")
    http.HandleFunc("/", handle)             // set router
    err := http.ListenAndServe(":8080", nil) // set listen port
    if err != nil {
        log.Fatal("ListenAndServe: ", err)
    }
}
