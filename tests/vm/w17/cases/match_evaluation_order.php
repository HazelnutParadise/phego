<?php
function t($label, $value)
{
    echo "[", $label, "]";
    return $value;
}
echo match (t("subject", 2)) {
    t("c1", 1) => t("b1", "one"),
    t("c2", 2) => t("b2", "two"),
    t("c3", 3) => t("b3", "three"),
    default => t("d", "default"),
}, "\n";
echo match (true) {
    t("a", false), t("b", false) => "ab",
    t("c", true), t("d", true) => "cd",
}, "\n";
echo match (t("s", 9)) {
    t("x", 1) => "x",
    default => t("def", "fallback"),
}, "\n";
$i = 0;
echo match (true) { $i++ > 5 => "a", $i++ > -1 => "b", $i++ > 0 => "c" }, $i, "\n";
echo match (1) { 1 => $u1, 2 => $u2 }, "|\n";
echo match (2) { 1 => $u1, 2 => "two" }, "\n";
