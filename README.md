# yabb
Yet Another Bonjour Browser

This Bonjour service browser displays on a console screen the services that it discovers. Changes are reflected as they occur.

The program is written in Go and uses andrewtj's dnssd package (https://github.com/andrewtj/dnssd) to query the DNS-SD system.

Provided you installed iTunes or the Bonjour SDK on your PC and despite the mention that the dnssd package is broken since Go 1.6, it appears to work seamlessly under Windows.

Under Linux, you'll have to install the libavahi-compat-libdnssd-dev package and add the GODEBUG=cgocheck=0 environment variable before running the program.
