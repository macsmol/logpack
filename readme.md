# logpack

Logpack is specialized compressor for log files. While on it's own it provides modest compression it can be used in tandem with dictionary based compressor (gzip, zstd).
For example it reaches compression ratios ~42% better than that of zstd alone (and will do it faster and using < 1 MB of RAM).

It is based on the ideas presented in "_Fast and efficient log file compression_" paper by Przemysław Skibiński and Jakub Swacha.

## Usage

### Packing
To compress `file.log` into `file.log.lp` run the following in the terminal:
```
logpack file.log
```
Compression level between `1` and `9` can be selected. Higher numbers will result in better compression but slow speeds. It is done in the following way:
```
logpack -8 file.log
```
### Unpacking
To unpack logpack archive `file.log.lp` run:
```
logpack -d file.log.lp
```

## What is it good for exactly?

Logpack will yield decent compression ratio (between 2-10x size reduction) for anything that looks like log file, eg:
```
[Thu Jun 09 06:07:04 2005] [notice] LDAP: Built with OpenLDAP LDAP SDK
[Thu Jun 09 06:07:04 2005] [notice] LDAP: SSL support unavailable
[Thu Jun 09 06:07:04 2005] [notice] suEXEC mechanism enabled (wrapper: /usr/sbin/suexec)
[Thu Jun 09 06:07:05 2005] [notice] Digest: generating secret for digest authentication ...
[Thu Jun 09 06:07:05 2005] [notice] Digest: done
[Thu Jun 09 06:07:05 2005] [notice] LDAP: Built with OpenLDAP LDAP SDK
[Thu Jun 09 06:07:05 2005] [notice] LDAP: SSL support unavailable
[Thu Jun 09 06:07:05 2005] [error] env.createBean2(): Factory error creating channel.jni:jni ( channel.jni, jni)
[Thu Jun 09 06:07:05 2005] [error] config.update(): Can't create channel.jni:jni
[Thu Jun 09 06:07:05 2005] [error] env.createBean2(): Factory error creating vm: ( vm, )
[Thu Jun 09 06:07:05 2005] [error] config.update(): Can't create vm:
[Thu Jun 09 06:07:05 2005] [error] env.createBean2(): Factory error creating worker.jni:onStartup ( worker.jni, onStartup)
```
Basically any repetitive text that contains space `' '` and newline  `\n` bytes is good. 

This makes it applicable to:

- Any contemporary text encodings (ASCII and derivative code pages, UTF-8, UTF-16 regardless of endianness)
- Windows/Linux/MacOS line endings
- Lines of reasonable length (smaller than 65kB)

Furthermore logpack is designed to swallow arbitrary input without crashing. So accidental oversized line or binary file should not be a problem. Such data will increase in size though[^1].

[^1]: Worst case compression doubles the size for typical input size. However the extreme case of compressing 1 non-ASCII byte will produce 6 bytes. The best case compression ratio is ~56x decrease.

## Benchmark results
Here are results of compression of samples from loghub corpus [^2] . See [here](#run-benchmarks) how to run benchmark.



### compression level: 4
|   		 | pack[MB/s] |    unpack[MB/s] |  compr. ratio [input/output]
|------------|------------|-----------------|----------------
| android_v1 |      66.67 |          347.32 |       2.092
| android_v2 |      78.63 |          337.74 |       2.410
|     apache |     245.17 |          249.22 |       9.021
|blue_gene_l |     319.91 |          345.67 |       5.259
|     hadoop |      83.37 |          551.04 |       1.495
|    hdfs_v1 |     114.34 |          348.95 |       3.814
|    hdfs_v2 |     435.96 |          385.19 |       5.363
| health_app |      62.61 |          483.31 |       1.571
|        hpc |     306.87 |          227.57 |       7.522
|      linux |     168.08 |          247.68 |       5.740
|        mac |     101.60 |          351.31 |       2.787
| open_stack |     171.36 |          433.91 |       3.535
|  proxifier |     113.72 |          280.87 |       5.299
|      spark |     145.60 |          247.81 |       7.681
|        ssh |     202.06 |          261.07 |       8.640
|thunderbird |     124.81 |          271.41 |       4.826
|    windows |     388.81 |          427.76 |       6.052
|  zookeeper |     176.31 |          276.68 |      10.71

### compression level: 9

|  		     | pack[MB/s] |    unpack[MB/s] |  compr. ratio [input/output]
|------------|------------|-----------------|----------------
| android_v1 |      16.55 |          326.64 |       2.436
| android_v2 |      18.18 |          320.53 |       2.836
|     apache |      31.87 |          222.97 |      11.43
|blue_gene_l |      20.55 |          326.14 |       5.795
|     hadoop |      33.39 |          319.96 |       8.932
|    hdfs_v1 |      17.54 |          307.47 |       4.969
|    hdfs_v2 |      43.48 |          324.86 |       7.825
| health_app |      17.41 |          480.17 |       1.610
|        hpc |      16.66 |          213.80 |       8.346
|      linux |      17.33 |          235.57 |       6.471
|        mac |      22.83 |          328.80 |       3.523
| open_stack |      39.55 |          424.49 |       3.865
|  proxifier |      23.32 |          250.17 |       6.751
|      spark |      19.01 |          217.27 |       9.907
|        ssh |      20.13 |          249.95 |       9.627
|thunderbird |      23.62 |          251.71 |       5.608
|    windows |      70.37 |          420.84 |       6.254
|  zookeeper |      19.79 |          254.07 |      12.69

### Compression improvement over ZSTD
Compression ratios mean `input_size / output_size`.

|             | zstd (ratio)|   lp(9)+zstd    |  improvement
|-------------|-------------|-----------------|------------------
|  android_v1 |   13.48     |       15.71     |        1.166         
|  android_v2 |   13.87     |       17.47     |        1.259         
|      apache |   21.95     |       30.93     |        1.409         
| blue_gene_l |    9.170    |       17.63     |        1.923         
|      hadoop |  178.8      |      211.3      |        1.182          
|     hdfs_v1 |   12.57     |       16.05     |        1.277         
|     hdfs_v2 |   19.13     |       24.88     |        1.300         
|  health_app |   11.02     |       12.40     |        1.125         
|         hpc |   12.29     |       22.47     |        1.828         
|       linux |   13.13     |       19.28     |        1.469         
|         mac |   20.11     |       25.63     |        1.275         
|  open_stack |   12.27     |       15.43     |        1.257         
|   proxifier |   15.97     |       18.65     |        1.168         
|       spark |   16.65     |       25.19     |        1.512         
|         ssh |   17.51     |       34.23     |        1.955         
| thunderbird |   17.64     |       30.22     |        1.713         
|     windows |   69.45     |      100.9      |        1.453         
|   zookeeper |   25.94     |       46.81     |        1.805      
|CORPUS_TOTAL |   16.14     |       22.95     |        1.422


[^2]: The results are based on <=10 MB samples from [LogHub corpus] (https://github.com/logpai/loghub/tree/master).

## Build and test
All the commands to be run from the repo root.

### Build executable:
```
go build .
```
### Run tests:
```
go test .\pack -v
```
### Run benchmarks:
Benchmarks
```
go test ./pack -v -run=ThisRegexMatchesNoTest  -bench=Packing$
```
Pit it against zstd:
```
go test ./pack -v -run=ThisRegexMatchesNoTest  -bench=Zstd$
```

## Need something better?
If logpack does not cut it you may be interested in LogpackPro. Here are few of it's highlights:
- Even better compression
- Streaming compression mode (compress logs in real time as they appear)
- Up to several times faster thanks to usage of SIMD instructions and other optimizations
- Available as a standalone program and C static library

Interested? Send me business inquiries on my LinkedIn (link in my GitHub profile)

