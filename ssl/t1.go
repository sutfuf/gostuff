package main

// Build: go build t1.go

import (
	"fmt"
	"crypto/x509"
	"os"
	"flag"
	"net/http"
	"net/http/httputil"
	"crypto/tls"
	"encoding/pem"
	"strings"
	"path/filepath"
	"syscall"
	"time"
)


// read_file fileName
// Read entire contents of fileName into byte array. 
// Warning on large files: duh. 
func read_file(fileName string) []byte {
	data,rc:=os.ReadFile(fileName)
	if (rc!=nil) {
		panic(rc)
		os.Exit(1)
	}
	return data
}

// createSSLTransport
// return *http.Transport object ot caller.
// warning: can skip verification (using this to download certs) using -k switch
// yeah, lets just copy curls args so I remember them. ;-) 
func createSSLTransport(certPool *x509.CertPool) *http.Transport {

	tlsClientConfig:=&tls.Config {
		RootCAs:certPool,
		InsecureSkipVerify:*insecureFlag,
	}

	tr:=&http.Transport{
		TLSClientConfig:tlsClientConfig,
	}
	return tr
}

// getThing (url -> string of url, *http.Transport)
// saves response in . (save_response -> testing out channels)
func getThing(url string, httpTransport *http.Transport ) {
	fmt.Println("Creating client")
	client := &http.Client{Transport: httpTransport}
	fmt.Println("Sending request")
	resp, rc := client.Get(url)
	if (rc!=nil) {
		panic(rc)
	}
//	fmt.Println(resp.Body.ReadAll())
	defer resp.Body.Close()

	fmt.Printf("%s %s\n",resp.Status,resp.Proto)
	fmt.Println(resp.Header)
	fmt.Println(resp.ContentLength)
	if (resp.TLS!=nil) {	 // we have ssl connection, process it
		fmt.Println(resp.TLS)
		processPeerCerts(resp.TLS.PeerCertificates)
	}
	save_response(resp, ".")
}

// putThing 
// put filename to purl (No idea if this works)
// need better file reader -> 8k chunks. an nmapper would be fun too
func putThing(url string, httpTransport *http.Transport, fileName string) {

	fileObject,rc:=os.Open(fileName)
	if (rc!=nil) {
		fmt.Printf("Error opening file: [%s]\n", fileName)
		panic(rc)
		os.Exit(1)
	}

	// fileInfoObject,rc:=fileObject.Stat()
	_,rc=fileObject.Stat()
	if (rc!=nil) {
		fmt.Printf("Cannot stat file: [%s]\n",fileName)
		panic(rc)
		os.Exit(1)
	}
//	statObject.size()  // filesize
	
	putUrl:=url+"/"+fileName

	client:=&http.Client{Transport: httpTransport}
	request,rc:=http.NewRequest("PUT", putUrl, fileObject) // b?
	if (rc!=nil){
		fmt.Printf("Cannot create request\n")
		panic(rc)
		os.Exit(1)
	}
	rd,rc:=httputil.DumpRequest(request, false)
	if (rc!=nil) {
		fmt.Printf("Cannot dump request\n")
		panic(rc)
		os.Exit(1)
	}
	fmt.Println(string(rd))
	response,rc:=client.Do(request)
	if (rc!=nil) {
		fmt.Printf("Fail")
		panic(rc)
		os.Exit(1)
	}
	fmt.Printf("Response code: [%d]\n",response.StatusCode)

}


// processPeerCerts
// Save peer certs (issuers and the like)
// save them into their respective common names.
// Replace all " " (spaces) with "_" underscores because spaces are so erky. 
func processPeerCerts(peerCerts []*x509.Certificate) {
	for _,certificate:=range peerCerts {
		fmt.Printf("Issuer: [%s]\n",certificate.Issuer.CommonName)
		fmt.Printf("Subject: [%s]\n",certificate.Subject.CommonName)
		block:=&pem.Block{Bytes:certificate.Raw,Type:"CERTIFICATE"}
		fileNameFixed:=strings.ReplaceAll(certificate.Subject.CommonName," ","_")
		fileNameFixed=fileNameFixed+".pem"
		certFile,rc:=os.OpenFile(fileNameFixed,os.O_CREATE|os.O_RDWR,0644)
		if(rc!=nil){
			fmt.Println(rc)
		}
		rc=pem.Encode(certFile, block)
		if (rc!=nil){
			fmt.Println(rc)
		certFile.Close()
		}
	}
}

// save_response
// given a response object, save it's contents to destination_path (string) -> basename URL
// the filename will be the end bit of the URL (forgot it's actual name)
// Testing: channels. :-) go is pretty cool. Pity about the missing macros and
// fucked up statements. I like my {squiggy brackets} to LINE UP! grrr.. 
// hmm.. what to do when basename is null? i.e.: blah.blah.x/ -> index.html? 
// need to check the response for the file that was given to us and rpoicess it. 
// meh, todo. 

func save_response ( resp *http.Response, destination_path string) {
	contentLength:=resp.ContentLength
	buffer:=make([]byte,8192)	// I really miss #define :-/  const (blah) is lame
	fileBaseName:=filepath.Base(resp.Request.URL.Path)
	filePathName:=filepath.Join(destination_path,fileBaseName)
	fmt.Printf("Saving file: [%s][%d]\n",filePathName,contentLength)

	fd_dest,rc:=syscall.Open(filePathName,
		syscall.O_RDWR|syscall.O_TRUNC|syscall.O_CREAT,
		syscall.S_IRUSR|syscall.S_IWUSR)

	if (rc!=nil) {
		fmt.Printf("Cannot open destination file for writing: [%s]\n", filePathName)
		panic(rc)
		os.Exit(1)
	}

	// we know length... we have block size = 8192...
	// so, content-length/blocksize
	// this give is the total number of updates.
	// We want to pin this at 20.
	// so, (content-len/blocksize)/20 -> total to expect _per_ println.
	// 
	updateSize:=int((contentLength/8192)/20) // really need to check out that const thing

	defer syscall.Close(fd_dest)	// close me later i.e.: when i return. :-) noice! 
	// channel to send bytes read

	bytesReadChannel:=make(chan int)
	defer close(bytesReadChannel)
	var allDataRead int
	var totalRead int
	// goroutine to print download status
	go func() {
		startTime:=time.Now()
		for dataRead:=range bytesReadChannel {
			allDataRead+=dataRead
			totalRead+=dataRead
			if (allDataRead>(updateSize*8192)) {
				latestTime:=time.Now()
				deltaTime:=latestTime.Sub(startTime)
				rate:=totalRead/int((deltaTime.Seconds()))
				fmt.Printf("Data Read: [%d][%d][%d](%d)B/s\n",dataRead,updateSize,totalRead,rate)
				allDataRead=0 
			}
		}
	}()

	for {
		bytesRead,rc:=resp.Body.Read(buffer)
		if (rc!=nil) {
			if (bytesRead>0) {
				bytesReadChannel<-bytesRead
				_,rc=syscall.Write(fd_dest,buffer[:bytesRead])
				if (rc!=nil) {
					fmt.Printf("Error writing (%d) to file [%s]\n", bytesRead, filePathName)
					panic(rc) 
					os.Exit(1) // screw you guys, i'm going home
				}
			}
			break
		}
		bytesReadChannel<-bytesRead
		_,rc=syscall.Write(fd_dest,buffer[:bytesRead])
		if (rc!=nil) {
			fmt.Printf("Error writing (%d) to file [%s]\n", bytesRead, filePathName)
			panic(rc)
			os.Exit(1) // talking poo is where i draw the line
		}
	}
}

// curl rules.
var insecureFlag = flag.Bool("k",false,"Skip SSL verify")

// ./t1 -f chain.pem -u www.google.com

func main() {

	var pemFileFlag = flag.String("f", "", "pem file") 	// for test ssl verification. (chain file)
	var urlFlag = flag.String("u","","url") // ok  this lame. Need to just specify url. 

	flag.Parse()
	url:=*urlFlag
	fmt.Printf("Processing file: [%s]\n",*pemFileFlag)
	certPool:=x509.NewCertPool()
	pemData:=read_file(*pemFileFlag)
	certPool.AppendCertsFromPEM(pemData)

	// for _cert:=range certPool {
	// 	fmt.Println("Certificate: ")
	// 	fmt.Println(_cert)
	// }

	fmt.Println("Creating Transport")
	tr:=createSSLTransport(certPool)
	getThing(url,tr)

//	putThing(url,tr,"test.txt")

}

