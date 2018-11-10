package main

var (
    ValidTags = []string{"sushi", "burger", "cheap", "fast", "close"}
    Locations = []Location{
        {
            Name: "Chuck's",
            Tags: map[string]bool{
                "burger": true,
                "fast":   true,
            },
            Capacity: 6,
        },
        {
            Name: "Oak City Meatball Shop",
            Tags: map[string]bool{
                "close": true,
            },
            Capacity: 4,
        },
        {
            Name: "Woody's",
            Tags: map[string]bool{
                "close":  true,
                "cheap":  true,
                "burger": true,
            },
            Capacity: 6,
        },
        {
            Name: "Vic's",
            Tags: map[string]bool{
                "close": true,
                "cheap": true,
            },
            Capacity: 8,
        },
        {
            Name: "El Dorado",
            Tags: map[string]bool{
                "close": true,
                "cheap": true,
            },
            Capacity: 8,
        },
        {
            Name:     "Sitti",
            Tags:     map[string]bool{},
            Capacity: 8,
        },
        {
            Name:     "Centro",
            Tags:     map[string]bool{},
            Capacity: 4,
        },
        {
            Name:     "Jose and Sons",
            Tags:     map[string]bool{},
            Capacity: 6,
        },
        {
            Name:     "The Pit",
            Tags:     map[string]bool{},
            Capacity: 8,
        },
        {
            // not a typo
            Name:     "Caffe Luna",
            Tags:     map[string]bool{},
            Capacity: 6,
        },
        {
            Name: "Raleigh Times",
            Tags: map[string]bool{
                "burger": true,
            },
            Capacity: 6,
        },
        {
            Name:     "Capital Club",
            Tags:     map[string]bool{},
            Capacity: 4,
        },
        {
            Name:     "Bida Manda",
            Tags:     map[string]bool{},
            Capacity: 6,
        },
    }
)

type Location struct {
    Name string
    Tags map[string]bool
    // how large a group can easily go here
    Capacity int
    // how large a group can easily go here at peak times
    PeakCapacity int
}

type History map[string]Visit

type Visit struct {
    Count int
    // millis
    LastVisitDate int
}
