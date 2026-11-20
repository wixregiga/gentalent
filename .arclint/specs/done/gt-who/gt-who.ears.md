# gt-who: requirements

Each line names the domain key it refers to. The rule text lives only in the domain recording.

- When a Member sends `!!gtwho <description>`, GenTalent shall start a WhoSearch only within the Group's who search daily limit. Domain: nominations / Group / assertion / who-searches-within-daily-limit
- When a WhoSearch's AI answer names a public figure, GenTalent shall reply with that public figure as the Person the Group already holds when the name spells one. Domain: nominations / Group / assertion / public-figure-spelled-as-a-person-is-that-person
- When a WhoSearch's AI answer arrives, GenTalent shall reply with one public figure, a short blurb and one photo, and keep any source link out of the reply. Domain: nominations / Group / entity / WhoSearch
