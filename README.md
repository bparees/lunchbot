# lunchbot
a slackbot for picking lunch locations

Interact w/ the bot with commands of the form:

    @lunchbot [attr1, attr2, attr3] lunch [for N people]

e.g.

* `@lunchbot lunch`
* `@lunchbot fast, cheap lunch`
* `@lunchbot lunch for 3 people`
* `@lunchbot fast, cheap lunch for 3 people`

The bot will do its best to pick a location that has those attributes
and can normally seat that many people.

## TODO

* Account for time of day and use PeakCapacity when determining whether the location can handle the group size
* Record history of selections on disk and weight that when randomizing the location selection
* Add a veto/reject command that will 
  * Unrecord the previous selection from the history
  * Pick a new location using the same parameters as the previous request
* Add day of week logic to avoid picking locations that are closed on a given day