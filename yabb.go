package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/andrewtj/dnssd"
)

// A browsed (and possibly resolved) service.
type brsT struct {
	name string				// host name
	host string				// host address (e.g. 'PC_REMI3.local.')
	port int				// host port (e.g. 80)
	txt map[string]string	// TXT records
	rop *dnssd.ResolveOp	// current ResolveOp
}

// All the services.
type serviceT struct {
	stype string			// service type (e.g '_http._tcp.')
	domain string			// domain (e.g. 'local.')
	bop *dnssd.BrowseOp		// current BrowseOp
	brs []brsT				// browsed and resolved services
}
var services []serviceT
var	serviceMutex sync.Mutex
var serviceChange chan struct{}	// channel to notify changes (could be chan bool as well)

// Display the services that have been detected.
func displayServices() {
	fmt.Println()
	log.Printf("%d services:\n", len(services))
	for _, s := range services {
		fmt.Printf("  %s%s\n", s.stype, s.domain)
		for _, b := range s.brs {
			fmt.Printf("    * %s: host = %s, port = %d\n", b.name, b.host, b.port)
			// Show the TXT record content
			if b.txt != nil && len(b.txt) != 0 {
				for mk, mv := range b.txt {
					fmt.Printf("      . %s: %s\n", mk, mv)
				}
			}
			if b.rop == nil {
				fmt.Println("      !NO rop!")
			}
		}
	}
}
// This goroutine waits for changes in the services slice and display its content.
// In case of burst, only take the last change into account.
func displayChanges() {
	t := time.NewTimer(time.Minute)	// create the *Timer before using it!
	tactive := true
	for {
		select {
		case _, ok := <- serviceChange:
			if !ok {	// channel closed
				return
			}
			if true {	// some change has occured
				if !tactive {	// no timer is currently active
					t = time.NewTimer(250 * time.Millisecond)	// *Timer
					tactive = true
				} else { // a timer is currently active
					if !t.Stop() {
						<-t.C
					}
					t.Reset(250 * time.Millisecond)
				}
			}
		case <- t.C:
			serviceMutex.Lock()
			displayServices()
			serviceMutex.Unlock()
			tactive = false
		}
	}
}

// Search a given service in the services slice.
// Return the index in services or -1
func serviceIndex(stype, domain string) int {
	for i, s := range services {
		if stype == s.stype && domain == s.domain {	// found!
			return i
		}
	}
	return -1
}

// Extract the strings of an RR data.
// src is a slice of bytes such as [len1, <len1 bytes>, len2, <len2 bytes>, ..., 0]
func xtractStrings(src []byte) (res []string) {
	for i := 0; i != len(src); {
		n := int(src[i])	// len of what follows
		i++
		if n == 0 || i >= len(src) {
			break
		}
		res = append(res, string(src[i:i + n]))
		i += n
	}
	return
}

// Resolve callback, invoked when a service's characteristics have been resolved.
func resolveCallback(rop *dnssd.ResolveOp, err error, host string, port int, txt map[string]string) {
	if err != nil {	// rop is now inactive
		log.Printf("Resolve operation failed: %s\n", err)
		return
	}
//log.Printf("\nResolved service %s to host %s and port %d with meta info: %v\n", rop.Type(), host, port, txt)
	var q struct{}
	defer func(){serviceChange <- q}()	 	// finally notify the change in services

	
	// Find the service that has been resolved in our services array.
	// Both the service and a valid brs should exist.
	stype := rop.Type()
	domain := rop.Domain()
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	
	i := serviceIndex(stype, domain)
	if i == -1 {
		fmt.Printf("\nSERVICE '%s%s' EXPECTED IN services BUT NOT FOUND!\n", stype, domain)
		return
	}
	
	// There should be a brsT in brs that matches the host machine name.
	name := rop.Name()
	for j, brs := range services[i].brs {
		if name == brs.name {	// found!
			services[i].brs[j].host = host
			services[i].brs[j].port = port
			services[i].brs[j].txt = txt
			services[i].brs[j].rop = rop
//fmt.Println("Resolved service: brs =", brs)
			break
		}
	}
}

// Browse callback, invoked when a service is discovered or abandoned.
func browseCallback(bop *dnssd.BrowseOp, err error, add bool, interfaceIndex int, name string, serviceType string, domain string) {
	if err != nil { // bop is now inactive
		log.Printf("Browse operation failed: %s\n", err)
		return
	}
	
	serviceMutex.Lock()
	defer serviceMutex.Unlock()
	
	// Find the big service matching serviceType and domain.
	is := serviceIndex(serviceType, domain)
	if is == -1 {	// not found!
		fmt.Printf("browseCallback: MISSING '%s/%s'!!!\n", serviceType, domain)
		return
	}
	
	if add { // a service has been discovered
//log.Printf("browseCallback: discovered '%s%s' on %s\n", serviceType, domain, name)
		
		// We have a new browsed service.
		// Set its host name and append it to its brs slice.
		var brs brsT
		brs.name = name
		services[is].brs = append(services[is].brs, brs)

		// Finally initiate the resolve operation.
		rop := dnssd.NewResolveOp(interfaceIndex, name, serviceType, domain, resolveCallback)
		err := rop.Start()
		if err != nil {	// rop is now inactive
			log.Printf("Failed to start resolve operation: %s\n", err)
			// Remove the freshly appended structure...
			services[is].brs = services[is].brs[:len(services[is].brs) - 1]	// last element
		}
	} else {	// a service had been abandoned
//log.Printf("browseCallback: service '%s%s' on %s has gone (%d)...\n", serviceType, domain, name, is)
		
		// Remove the browsed service from its slice.
		for ibrs, brs := range services[is].brs {
			if brs.name == name	{	// bingo!
				brs.rop.Stop()
//				ns := services[is].brs[:ibrs]	// [0..ibrs - 1]
//				services[is].brs = append(ns, services[is].brs[ibrs + 1:]...)
				s := services[is].brs
				copy(s[ibrs:], s[ibrs + 1:])
				services[is].brs = s[:len(s) - 1]	// update slice length
				break
			}
		}
		
		// If the slice is empty, remove the service entry.
		if len(services[is].brs) == 0 {
			services[is].bop.Stop()		// cancel the BrowserOp
//			ns := services[:is]	// [0..is - 1]
//			services = append(ns, services[is + 1:]...)
			copy(services[is:], services[is + 1:])
			services = services[:len(services) - 1]	// update slice length
		}

		// Notify the change in services.
		var q struct{}
		serviceChange <- q
	}
//fmt.Println("browseCallback: services =", services)
}

// Query callback.
// We initiate a BrowseOp for the given service type and domain.
func queryCallback(qop *dnssd.QueryOp, err error, add bool,
					interfaceIndex int, fullname string, rrtype, rrclass uint16,
					   rdata []byte, ttl uint32) {
	if err != nil {	// qop is now inactive
		log.Printf("Query operation failed: %s\n", err)
		return
	}
//fmt.Printf("\nqueryCallback: add = %v, interfaceIndex = %d, fullname = '%s', rrtype = %d, rrclass = %d\nrdata = %v, ttl = %d\n\n", add, interfaceIndex, fullname, rrtype, rrclass, rdata, ttl)

	// There is an operation on a service, whose characteristics are in rdata.
	str := xtractStrings(rdata)

	if add {
		var s serviceT
		s.stype = str[0] + "." + str[1] + "."	// e.g. "_http._tcp."
		s.domain = str[2] + "."					// e.g. "local."
//log.Printf("queryCallback: ADDED '%s%s'\n", s.stype, s.domain)
	
		// Initiate a BrowseOp for this service type and domain.
		op, err := dnssd.StartBrowseOp(s.stype, browseCallback)
		if err != nil {	// op is now inactive
			log.Printf("Failed to start browse operation: %s\n", err)
			return
		}
		s.bop = op	// save the BrowseOp for final close
//fmt.Println("Created service", s)
	
		services = append(services, s)
	} else {
//log.Printf("queryCallback: REMOVED '%s.%s.%s.'\n", str[0], str[1], str[2])
	}
}

// Initiate the global query of all services.
// Use the magic name "_services._dns-sd._udp", kDNSServiceType_PTR (12) and
// kDNSServiceClass_IN (1) for rrtype and rrclass. 
func queryAllServices() (op *dnssd.QueryOp, err error) {
	op, err = dnssd.StartQueryOp(0, "_services._dns-sd._udp.local.", 12, 1, queryCallback)
	if err != nil {
		log.Printf("Failed to start query operation: %s\n", err)
	}
	return // op and err are always returned
}

// Clean pending DNS-SD Ops.
func stopAllOps() {
	for _, s := range services {
		s.bop.Stop()
		for _, brs := range s.brs {
			brs.rop.Stop()
		}
	}
}

// *** The main function ***
func main() {
	fmt.Println("*** Yet Another Bonjour Browser\n")
	fmt.Println("Hit Ctrl-C to exit program\n")
	
	// Start with no service.
	services = services[:0]

	// Initiate the global query of all services.
	qop, err := queryAllServices()
	if err != nil {
		return
	}
	defer qop.Stop()			// clean the global QueryOp
	defer stopAllOps()			// clean potentially pending Ops in services slice
	
	serviceChange = make(chan struct{})	// create the global channel
	defer close(serviceChange)	// close the global channel

	// Start the goroutine in charge with the changes in services.
	go displayChanges()
	
	// Do nothing, without monopolizing the CPU.
	for {
		time.Sleep(1000 * time.Millisecond)
	}
}