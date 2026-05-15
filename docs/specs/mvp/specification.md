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

## How Herald should achieve its mission objectives?

Tbd

## Flavors (the implementations)

Tbd

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
