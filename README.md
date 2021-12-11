# Twitter ID Finder

n文字の全数IDを探せます

## Usage

```
$ make
go build -o main main.go

$ ./main
Twitter ID Finder
Creator: @_m_vt

Digits: 2
Target IDs: 100
Really? [Y/n]: 
100 / 100 [----------------------------------------------------------------] 100.00% 95 p/s 1.2s
Available IDs: 0 / 100
```

## proxies.txt

https://pkg.go.dev/net/url#URL のURL形式を用いてプロキシを行毎に記述してください。