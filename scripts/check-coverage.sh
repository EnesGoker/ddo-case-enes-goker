#!/usr/bin/env bash
# Profildeki statement ağırlıklarıyla saf domain paketlerinin kapsamını denetler.
set -euo pipefail
profile="${1:-backend/coverage.out}"
awk '
BEGIN {
 n=split("billing caldate pricing proration settlement fraud workflow lossratio", names, " ")
 print "| Domain paketi | Kapsam | Sonuç |\n|---|---:|---|"
}
NR>1 {
 split($1,parts,"/internal/");split(parts[2],path,"/")
 total[path[1]]+=$2
 if ($3>0) covered[path[1]]+=$2
}
END {
 for(i=1;i<=n;i++) {
  p=names[i]; pct=total[p]>0?100*covered[p]/total[p]:0
  printf "| %s | %.1f%% | %s |\n",p,pct,(pct>=70?"PASS":"FAIL")
  if(pct<70) bad=1
 }
 exit bad
}' "$profile"
