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
    "sync"
    "time"
)

type DepartureTime struct {
    Hour   int
    Minute int
}

func (d DepartureTime) String() string {
    return fmt.Sprintf("%d:%d", d.Hour, d.Minute)
}

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
    User    string `json:"user"`
    Channel string `json:"channel"`
    TS      string `json:"ts"`
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
        if _, found := msgCache[req.Event.TS]; found {
            fmt.Printf("ignoring dupe event: %#v\n", req.Event)
        }
        msgCache[req.Event.TS] = struct{}{}
        fmt.Printf("saw message event: %#v\n", req.Event)

        msg := PostMessage{}
        //msg.Token = auth_token
        msg.Channel = req.Event.Channel

        switch {
        case strings.Contains(req.Event.Text, "help"):
            msg.Text = DoHelp()
        case strings.Contains(req.Event.Text, "lunch"):
            msg.Text = DoLunch(req.Event.Text)
        case strings.Contains(req.Event.Text, "rollcall"):
            msg.Text = DoRollCall(req.Event.Text)
        case strings.Contains(req.Event.Text, "reset"):
            msg.Text = DoReset()
        case strings.Contains(req.Event.Text, "<@UE23Q9BFY> in"):
            msg.Text = HandleRollCallResponseIn(req.Event.Text, req.Event.User)
        case strings.Contains(req.Event.Text, "<@UE23Q9BFY> out"):
            msg.Text = HandleRollCallResponseOut(req.Event.User)
        default:
            msg.Text = fmt.Sprintf("Sorry, I couldn't process that request: %s", req.Event.Text)
        }

        // never output our own name, so we don't trigger ourselves
        //fmt.Printf("original response: %s\n", msg.Text)
        msg.Text = strings.Replace(msg.Text, "<@UE23Q9BFY>", "lunchbot", -1)
        //fmt.Printf("replaced response: %s\n", msg.Text)

        msgJson, _ := json.Marshal(msg)

        fmt.Printf("msg response json: %s\n", msgJson)
        req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(msgJson))
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", auth_token))

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil {
            fmt.Printf("error posting chat message: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
        }
        //fmt.Printf("chat message response: %#v\n", resp)
        resp.Body.Close()

        w.WriteHeader(http.StatusOK)

        //respJson, _ := json.Marshal(resp)
        //io.WriteString(w, string(respJson))
    }
}

func DoHelp() string {
    return helpText
}
func DoRollCall(input string) string {
    mutex.Lock()
    defer mutex.Unlock()
    if rollCallInProgress {
        c, d := Count()
        return fmt.Sprintf("There is already a rollcall in progress.  The participant count is %d and the departure time is %d:%02d", c, d.Hour, d.Minute)
    }
    Reset()
    rollCallInProgress = true
    // reset the counts after 3 hours, so we're ready for the next day.
    timer := time.NewTimer(180 * time.Minute)
    go func() {
        <-timer.C
        DoReset()
    }()

    return "<!here> If you're coming to lunch, please respond with your earliest availability in the form: `@lunchbot in HH:MM`.  If you do not specify a time, 11:30 is assumed."
}

func DoReset() string {
    mutex.Lock()
    defer mutex.Unlock()
    //participantCount = 0
    //departureTime = DepartureTime{11, 30}
    //participants = make(map[string]DepartureTime)
    //rollCallInProgress = false
    Reset()
    return "The rollcall has been reset, to initiate a new rollcall please say `@lunchbot rollcall`"
}

func Reset() {
    participants = make(map[string]DepartureTime)
    rollCallInProgress = false
    msgCache = make(map[string]struct{})
}

func HandleRollCallResponseIn(input, sender string) string {
    mutex.Lock()
    defer mutex.Unlock()
    if !rollCallInProgress {
        return fmt.Sprintf("<@%s> no rollcall is in progress, you can start one by saying `@lunchbot rollcall`", sender)
    }

    matches := rollcallparser.FindStringSubmatch(input)
    if len(matches) == 0 {
        return fmt.Sprintf("Sorry <@%s>, I could not parse your rollcall response: %s", sender, input)
    }
    participantTime := DepartureTime{11, 30}
    if len(matches) == 2 && len(matches[1]) > 0 {
        d := matches[1]
        bits := strings.Split(d, ":")
        h, _ := strconv.Atoi(bits[0])
        m, _ := strconv.Atoi(bits[1])
        if h < 11 || h > 13 {
            return fmt.Sprintf("<@%s>, please use 24-hour time with an hour of 11, 12, or 13.", sender)
        }
        if m < 0 || m > 59 {
            return fmt.Sprintf("<@%s>, %d is not a valid minute value.", sender, m)
        }
        participantTime.Hour = h
        participantTime.Minute = m
        /*
           if h > departureTime.Hour {
               departureTime.Hour = h
               departureTime.Minute = m
           } else if h == departureTime.Hour && m > departureTime.Minute {
               departureTime.Minute = m
           }
        */
    }

    _, exists := participants[sender]
    //participantCount += 1
    participants[sender] = participantTime
    count, departureTime := Count()

    if exists {
        return fmt.Sprintf("Thank you <@%s>, your response has been updated. The participant count is %d and the earliest departure is %d:%02d", sender, count, departureTime.Hour, departureTime.Minute)
    }
    return fmt.Sprintf("Thank you <@%s>, the new participant count is %d and the earliest departure is %d:%02d", sender, count, departureTime.Hour, departureTime.Minute)
}

func HandleRollCallResponseOut(sender string) string {
    mutex.Lock()
    defer mutex.Unlock()
    if !rollCallInProgress {
        return fmt.Sprintf("<@%s> no rollcall is in progress, you can start one by saying `@lunchbot rollcall`", sender)
    }

    _, exists := participants[sender]
    if exists {
        delete(participants, sender)
        count, departureTime := Count()
        return fmt.Sprintf("Thank you <@%s>, you have been removed from the list of participants. The participant count is %d and the earliest departure is %d:%02d", sender, count, departureTime.Hour, departureTime.Minute)
    }
    count, departureTime := Count()
    return fmt.Sprintf("<@%s>, you were not in the participant list.  The participant count is %d and the earliest departure is %d:%02d", sender, count, departureTime.Hour, departureTime.Minute)
}

func DoLunch(input string) string {
    locations, count, err := PickLocation(input)

    resp := ""
    if err != nil {
        resp = fmt.Sprintf("Sorry, I couldn't process that request: %v", err)
    } else {
        switch len(locations) {
        case 1:
            resp = fmt.Sprintf("For %d people I recommend %s", count, locations[0].Name)
        case 2:
            resp = fmt.Sprintf("For %d people I recommend %s or %s", count, locations[0].Name, locations[1].Name)
        case 3:
            resp = fmt.Sprintf("For %d people I recommend %s, %s, or %s", count, locations[0].Name, locations[1].Name, locations[2].Name)
        default:
            resp = fmt.Sprintf("Sorry, I couldn't find any suitable locations")
        }
    }
    participantList := ""
    for p := range participants {
        participantList = fmt.Sprintf("<@%s> ", p)
    }

    return fmt.Sprintf("%s it's time for lunch!  %s", participantList, resp)
}

func PickLocation(text string) ([]Location, int, error) {
    tags, groupSize, err := Parse(text)
    if err != nil {
        return []Location{}, groupSize, err
    }
    filteredLocations := FilterLocations(tags, groupSize)
    if len(filteredLocations) == 0 {
        return []Location{}, -1, fmt.Errorf("no locations matched the specified requirements")
    }
    if len(filteredLocations) <= 3 {
        return filteredLocations, groupSize, nil
    }

    results := []Location{}
    first := rand.Intn(len(filteredLocations))
    results = append(results, filteredLocations[first])
    second := -1
    for {
        c := rand.Intn(len(filteredLocations))
        if c != first && c != second {
            second = c
            results = append(results, filteredLocations[c])
        }
        if len(results) == 3 {
            break
        }
    }
    return results, groupSize, nil
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
        //fmt.Printf("candidates is now %q\n", candidates)
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
        c, _ := Count()
        groupSize = c
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
        for i := range tags {
            tags[i] = strings.TrimSpace(tags[i])
        }
    }

    return tags, groupSize, nil
}

func Count() (int, DepartureTime) {
    departureTime := DepartureTime{11, 30}
    count := 0
    for _, v := range participants {
        if v.Hour > departureTime.Hour {
            departureTime.Hour = v.Hour
            departureTime.Minute = v.Minute
        } else if v.Hour == departureTime.Hour && v.Minute > departureTime.Minute {
            departureTime.Minute = v.Minute
        }
        count += 1
    }
    return count, departureTime
}

var (
    backtick = "`"
    helpText = "To start a lunch rollcall, say `@lunchbot rollcall`\n" +
        "To respond to a rollcall, say `@lunchbot in` or `@lunchbot in HH:MM` to indicate your earliest availability\n" +
        "To remove yourself from a rollcall, say `@lunchbot out`\n" +
        "To reset a rollcall say `@lunchbot reset` (rollcalls automatically reset after 2 hours)\n" +
        "To request a location suggestion, say `@lunchbot lunch` (current rollcall count will be used for location selection)\n" +
        "To request a location with specific attributes, say `@lunchbot attr1, attr2 lunch`\n" +
        "To request a location for a specific number of participants, say `@lunchbot lunch for N people`\n"

    parser             = regexp.MustCompile(`<@UE23Q9BFY> (.*?)lunch(?: for )?(\d*)`)
    rollcallparser     = regexp.MustCompile(`<@UE23Q9BFY> in(?: *)(\d\d?:\d\d)?`)
    auth_token         string
    rollCallInProgress = false
    //participantCount   = 0
    participants = make(map[string]DepartureTime)
    msgCache     = make(map[string]struct{})
    //departureTime      = DepartureTime{11, 30}
    mutex = &sync.Mutex{}
)

// msg format:  tag1, tag2, tag3 lunch for 6 people

func main() {
    rand.Seed(time.Now().UTC().UnixNano())
    auth_token = os.Getenv("TOKEN")
    http.HandleFunc("/", handle)             // set router
    err := http.ListenAndServe(":8080", nil) // set listen port
    if err != nil {
        log.Fatal("ListenAndServe: ", err)
    }
}
