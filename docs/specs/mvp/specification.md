# Herald

Repository: `git@github.com:vasic-digital/Herald.git`

## Purpose

Ingesting system events and reliably fanning them out to multiple notification channels so every alert reaches the right destination without confusion.

### What Herald can do?

Herald is the mechanism which receives input from the source and sends it to one or more destinations.
Depending on the implementation (flavor) of the Herald sources and destinations could be various.
We can have single type or multiple type inputs and same for the outputs.

For example input can be the result of execution of the pipeline and the output sending of this result (some report for example) to certain output - messagin system which notifies subscribers!

Possibilities are not limited.

The structure of the System MUST be hierarchical so in the top levels (closest to the root) we have abstractions and base reusable, main shared mechanisms, while inside the `flvors` directory we MUST have all flavors - the implementations.

*Note:* We MUST NOT be obligated to follow this structure as many parent project specific flavors (which may be propriatery) may exist! We MUST make sure that such flexibility is possible!

## How Herald should achieve its mission objectives?

Every herald flavor will be individual binnary which users can add to the System path throguh the `.bashrc` or `.zshrc` (for example). Each Herald flavor will have shared set of commands and parameters while there will be as well commands and parameters specific to particular Herald flavors.

Herald applications are CLI binarries which are mainly designed for CI integration and various Pipelines. They can be easily incorporate for use with various AI CLI Agents as well or any similar use cases (triggering by Crond and so on ...).

Some Herald application names would be: `pherald` for the `Project Herald`, `sherald` for the `System Herald` and so on.

### Technology stack

Herald project and all flavors MUST BE writtn in Go.

## Flavors (the implementations)

Main Go abstractions and shared codebase (with shared implementations) will be used as the base which Flavors will inherit and build on top of it.

### Project Herald

Tbd

### System Herald

Tbd

### Others and misc

Tbd

## Integration into the Constitution

Tbd

## Notes

Tbd
