---
description: pasted GSC with no game named, loop without a wait
tags: [trigger]
max_turns: 4
allowed_tools: [Skill, Read, Glob, Grep]
---

game freezes the second this starts, no error. why?

```
function watch_power()
{
    self endon("death");
    while(1)
    {
        if(level flag::get("power_on"))
            self SetModel("p7_light_on");
    }
}
```
