# mail-sink

mail-sink is a utility program that implements a "black hole" function.
It listens on the named host (or address) and port.
It accepts Simple Mail Transfer Protocol (SMTP) messages from the network and discards them.

Original repo is archived.
This fork is updated to allow saving of attached files in the current directory.

Install with:

    go install github.com/agrinchenko/mail-sink@latest

Usage:

    $ ./mail-sink -h
    Usage of mail-sink:
    -H="localhost": hostname to greet with
    -i="localhost": listen on interface
    -p=25: listen port
    -v=false: log the mail body
    -s=false: save attached files to current dir

Example: `./mail-sink -p 20025 -v -s`
