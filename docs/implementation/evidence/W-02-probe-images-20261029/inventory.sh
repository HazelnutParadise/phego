set -eu
export LC_ALL=C
dpkg-query -W -f='${db:Status-Status}\t${binary:Package}\t${Version}\n' > /tmp/packages
sort /tmp/packages > /tmp/packages-sorted
tab=$(printf '\t')
while IFS="$tab" read -r status package version; do
  [ "$status" = installed ] || continue
  base=${package%%:*}
  path=/usr/share/doc/$base/copyright
  if [ ! -e "$path" ]; then
    printf '%s\t%s\t%s\tmissing\t-\t-\t-\n' "$package" "$version" "$path"
    continue
  fi
  resolved=$(realpath -e -- "$path") || { printf '%s\t%s\t%s\tunresolvable\t-\t-\t-\n' "$package" "$version" "$path"; continue; }
  case "$resolved" in
    /usr/share/doc/*|/usr/share/common-licenses/*) ;;
    *) printf '%s\t%s\t%s\toutside_allowed_path\t%s\t-\t-\n' "$package" "$version" "$path" "$resolved"; continue ;;
  esac
  if [ ! -f "$resolved" ]; then
    printf '%s\t%s\t%s\tnot_regular\t%s\t-\t-\n' "$package" "$version" "$path" "$resolved"
  elif [ ! -r "$resolved" ]; then
    printf '%s\t%s\t%s\tunreadable\t%s\t-\t-\n' "$package" "$version" "$path" "$resolved"
  else
    sum=$(sha256sum -- "$resolved")
    hash=${sum%% *}
    size=$(wc -c < "$resolved" | tr -d ' ')
    printf '%s\t%s\t%s\trecorded\t%s\t%s\t%s\n' "$package" "$version" "$path" "$resolved" "$size" "$hash"
  fi
done < /tmp/packages-sorted
