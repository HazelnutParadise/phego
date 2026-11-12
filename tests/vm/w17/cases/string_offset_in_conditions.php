<?php
$s = "a0-";
if ($s[0] == "a") {
    echo "starts a\n";
}
if ($s[1]) {
    echo "truthy 0?\n";
} else {
    echo "zero string is falsy\n";
}
if ($s[2] === "-") {
    echo "dash\n";
}
switch ($s[0]) {
    case "b":
        echo "b\n";
        break;
    case "a":
        echo "a\n";
        break;
    default:
        echo "default\n";
}
switch ($s[1]) {
    case 0:
        echo "loose zero\n";
        break;
    case "0":
        echo "string zero\n";
        break;
}
echo $s[0] == "a" && $s[2] == "-" ? "both" : "not both", "\n";
echo $s[5] ?? $s[0], "\n";
$x = isset($s[1]) ? $s[1] : "none";
echo $x, "\n";
while ($s[0] == "a") {
    $s = "b" . $s;
}
echo $s, "\n";
