#!/usr/bin/env bash

set -euo pipefail

output_dir=".reports"
cov_unfiltered_out_file="${output_dir}/coverage.unfiltered.out"
cov_out_file="${output_dir}/coverage.out"
cov_out_xml_file="${output_dir}/coverage.xml"
report_file="${output_dir}/coverage_summary.txt"

rm -rf ${output_dir}
mkdir -p ${output_dir}
echo "" > ${report_file}

echo "--->  Testing code..."

go test -count=1 -tags=integration -coverpkg=./... -covermode=count -coverprofile ${cov_unfiltered_out_file} ./... | tee ${output_dir}/std.out
grep -v -E -f .covignore ${cov_unfiltered_out_file} > ${cov_out_file}

total="$(go tool cover -func ${cov_out_file} | tail -n 1 |  grep -Eo '[0-9]+\.[0-9]+')"
subtotals+="\t${total}%"

echo -e "Total coverage:\t${total}%" | tee -a $report_file

# shellcheck disable=SC2002
cat ${output_dir}/std.out | go-junit-report > ${output_dir}/reports.xml
gocover-cobertura < ${cov_out_file} > ${cov_out_xml_file} --ignore-gen-files
gcov2lcov -infile=${cov_out_file} -outfile=${output_dir}/lcov.info
