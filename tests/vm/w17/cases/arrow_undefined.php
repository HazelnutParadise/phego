<?php
$f = fn () => $missing;
echo "created_without_warning\n";
echo "call1: ", $f() === null ? "null" : "other", "\n";
echo "call2: ", $f() === null ? "null" : "other", "\n";
$missing = "now defined";
echo "after_define: ", $f() === null ? "still null" : "seen", "\n";

$g = fn ($x) => $x . $absent;
echo "concat_with_undefined: ", $g("a"), "\n";

$present = "here";
$h = fn () => $present;
echo "present_no_warning: ", $h(), "\n";

$k = fn () => isset($notYet) ? "set" : "unset";
echo "isset_no_warning: ", $k(), "\n";

$lazy = fn () => $later ?? "coalesce_default";
echo "coalesce_no_warning: ", $lazy(), "\n";
$later = "defined later";
echo "coalesce_after_define: ", $lazy(), "\n";

$inner = fn () => fn () => $deep;
echo "nested_created_quietly\n";
echo "nested_call: ", $inner()() === null ? "null" : "other", "\n";
