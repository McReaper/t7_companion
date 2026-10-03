---
description: pasted gsc snippet, logic bug in a perk (debugging is the alternative)
tags: [trigger, holdout]
max_turns: 4
allowed_tools: [Skill, Read, Glob, Grep]
---

this compiles fine but perk_count is undefined for every player after the first one, what am i doing wrong

```
function init()
{
    level thread perk_watch();
}

function perk_watch()
{
    level waittill("connected", player);
    player.perk_count++;
}
```
