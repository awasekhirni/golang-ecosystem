# golang-ecosystem
Golang-ecosystem
copyright 2014-2019 Dr. Syed Awase Khirni


Using Go Modules to create Projects 
#creating a project using go modules 

go mod init project 

#adding dependencies 

go get github.com/julienschmidt/httprouter@v1.2

go get go.uber.org/zap@latest

#install/remoce dependency

go mod tidy


#executing the 

go run src/main.go 

#building 
go build src/main.go 

#testing 
go test 

#cleaning the cache 
go clean -cache -modcache -i -r 

#comptability with vendor director older versions of the go 
go mod vendor 

#fetch all the versions of specific go package 
go list -m -versions go.uber.org/zap 
// this would not work with vendor directory in place, remove the vendor directory if exists 

# download all the dependencies 
go mod download 

#
go env | grep "GOPROXY"

# checksum go packages 
go env | grep "GOSUMDB"
