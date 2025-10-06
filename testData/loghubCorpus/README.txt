This data is based on downloads listed at https://github.com/logpai/loghub/blob/master/README.md

Each original log archive was uncompressed, raw data was trimmed to 10 MB. Then this was trimmed to last newline char.
If original log archive contained more files then first different (in lexicographic sort) *.log was selected (Android_v2 case). 
If original log archive contained *.log files, all/most of them being smaller than 10MB the biggest file was chosen (Hadoop case, Spark case: first biggest than 10MB)
HDFS (HDFS_v3_TraceBench.zip) was excluded because it did not contain any raw text log files.



bestCase os synthetic input that results in the best compression ratio
Care was taken to preserve original neline encoding (files are set as binary in .gitattributes)

Folder names are all written in lower_snake_case to keep sorting between various test consistent on Linux/Windows.