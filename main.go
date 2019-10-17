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

type Participant struct {
    DepartureTime DepartureTime
    In            bool
    Name          string
}
type DepartureTime struct {
    Hour   int
    Minute int
}

func (d DepartureTime) String() string {
    return fmt.Sprintf("%d:%02d", d.Hour, d.Minute)
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

type UserLookupRequest struct {
    User string `json:"user"`
}

type User struct {
    Name string `json:"name"`
}
type UserLookupResponse struct {
    User User `json:"user"`
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

        mutex.Lock()
        if _, found := msgCache[req.Event.TS]; found {
            fmt.Printf("ignoring dupe event: %#v\n", req.Event)
            w.WriteHeader(http.StatusOK)
            mutex.Unlock()
            return
        }
        msgCache[req.Event.TS] = struct{}{}
        mutex.Unlock()
        fmt.Printf("saw message event: %#v\n", req.Event)

        msg := PostMessage{}
        //msg.Token = auth_token
        msg.Channel = req.Event.Channel

        switch {
        case strings.Contains(req.Event.Text, "help"):
            msg.Text = DoHelp()
        case strings.Contains(req.Event.Text, "snack check"):
            msg.Text = DoSnack(false)
        case strings.Contains(req.Event.Text, "snack"):
            msg.Text = DoSnack(true)
        case strings.Contains(req.Event.Text, " lunch"):
            msg.Text = DoLunch(req.Event.Text)
        case strings.Contains(req.Event.Text, " status"):
            msg.Text = DoStatus(req.Event.Text)
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
        msg.Text = strings.Replace(msg.Text, "@UE23Q9BFY", "lunchbot", -1)
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

func LookupUser(user string) string {
    fmt.Printf("looking up user: %s\n", user)

    req, err := http.NewRequest("GET", fmt.Sprintf("https://slack.com/api/users.info?user=%s&token=%s", user, auth_token), nil)
    //req.Header.Set("Content-Type", "application/json")
    //req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", auth_token))

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Printf("error posting user lookup message: %v\n", err)
        return ""
    }
    if resp.StatusCode != 200 {
        fmt.Printf("lookup response error: %s\n", resp.StatusCode)
        return ""
    }
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Printf("error reading lookup response body: %v\n", err)
        return ""
    }
    lookupResponse := UserLookupResponse{}
    fmt.Sprintf("response: %s", string(body))
    if err := json.Unmarshal([]byte(body), &lookupResponse); err != nil {
        fmt.Printf("error reading lookup response: %v\n", err)
        return ""
    }
    return lookupResponse.User.Name
}

func DoHelp() string {
    return helpText
}

func DoSnack(announce bool) string {
    response, err := http.Get("https://redhatmain.southernfoodservice.com/Menu/Weekly")
    if err != nil {
        return fmt.Sprintf("Sorry, I got an error retrieving the snack menu: %v", err)
    }
    defer response.Body.Close()
    contents, err := ioutil.ReadAll(response.Body)
    if err != nil {
        return fmt.Sprintf("Sorry, I got an error retrieving the snack menu: %v", err)
    }
    groups := snack.FindStringSubmatch(string(contents))
    if len(groups) < 2 {
        return "Sorry, I could not determine the snack, you can look at the menu here: https://redhatmain.southernfoodservice.com/Menu/Weekly"
    }
    if announce {
        return fmt.Sprintf("<!here> it's snack time, the snack is %s", groups[1])
    }
    return fmt.Sprintf("This week's snack is(was) %s", groups[1])
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

func DoStatus(input string) string {
    participantList := ""
    outList := ""
    for _, p := range participants {
        if p.In && p.Name != "" {
            participantList = fmt.Sprintf("%s(%s) %s", p.Name, p.DepartureTime, participantList)
        }
        if !p.In && p.Name != "" {
            outList = fmt.Sprintf("%s %s", p.Name, outList)
        }
    }

    r := fmt.Sprintf("The following people are in: %s\n", participantList)
    if len(outList) > 0 {
        r = fmt.Sprintf("%sThe following people are out: %s", r, outList)
    }
    return r
}

func DoReset() string {
    mutex.Lock()
    defer mutex.Unlock()
    Reset()
    return "The rollcall has been reset, to initiate a new rollcall please say `@lunchbot rollcall`"
}

func Reset() {
    participants = make(map[string]Participant)
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
    }

    _, exists := participants[sender]
    participants[sender] = Participant{
        In:            true,
        DepartureTime: participantTime,
    }
    if len(participants[sender].Name) == 0 {
        user := LookupUser(sender)
        fmt.Printf("found user: %s\n", user)
        p := participants[sender]
        p.Name = user
        participants[sender] = p
    }
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
    user := LookupUser(sender)
    fmt.Printf("found user: %s\n", user)
    p := Participant{
        In:   false,
        Name: user,
    }
    participants[sender] = p
    count, departureTime := Count()
    return fmt.Sprintf("Thank you <@%s>, you have been removed from the list of participants. The participant count is %d and the earliest departure is %d:%02d", sender, count, departureTime.Hour, departureTime.Minute)
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
    outList := ""
    for k, p := range participants {
        if p.In {
            participantList = fmt.Sprintf("<@%s> %s", k, participantList)
        }
        if !p.In && p.Name != "" {
            outList = fmt.Sprintf("%s %s", p.Name, outList)
        }
    }

    r := fmt.Sprintf("%s it's time for lunch!  %s\n", participantList, resp)
    if len(outList) > 0 {
        r = fmt.Sprintf("%sFYI the following people are out: %s", r, outList)
    }
    return r
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
        if !v.In {
            continue
        }
        if v.DepartureTime.Hour > departureTime.Hour {
            departureTime.Hour = v.DepartureTime.Hour
            departureTime.Minute = v.DepartureTime.Minute
        } else if v.DepartureTime.Hour == departureTime.Hour && v.DepartureTime.Minute > departureTime.Minute {
            departureTime.Minute = v.DepartureTime.Minute
        }
        count += 1
    }
    return count, departureTime
}

var (
    helpText = "To start a lunch rollcall, say `@lunchbot rollcall`\n" +
        "To respond to a rollcall, say `@lunchbot in` or `@lunchbot in HH:MM` to indicate your earliest availability\n" +
        "To remove yourself from a rollcall, say `@lunchbot out`\n" +
        "To reset a rollcall say `@lunchbot reset` (rollcalls automatically reset after 2 hours)\n" +
        "To request a location suggestion, say `@lunchbot lunch` (current rollcall count will be used for location selection)\n" +
        "To request a location with specific attributes, say `@lunchbot attr1, attr2 lunch`\n" +
        "To request a location for a specific number of participants, say `@lunchbot lunch for N people`\n" +
        "To check the current participant responses, say `@lunchbot status`\n" +
        "To check on the weekly snack, say `@lunchbot snack check`\n" +
        "To notify the channel about the snack, say `@lunchbot snack`\n"

    parser             = regexp.MustCompile(`<@UE23Q9BFY> (.*?)lunch(?: for )?(\d*)`)
    rollcallparser     = regexp.MustCompile(`<@UE23Q9BFY> in(?: *)(\d\d?:\d\d)?`)
    snack              = regexp.MustCompile("SNACK.*?\"name\":\"(.*?)\"")
    auth_token         string
    rollCallInProgress = false
    //participantCount   = 0
    participants = make(map[string]Participant)
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
