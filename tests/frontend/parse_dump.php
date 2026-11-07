<?php
// Keep both observed locations. A call-relative error follows these two
// distinct token_get_all call sites; an ordinary source error stays put.
function parse_first($src) {
    try {
        $call = __LINE__ + 1;
        token_get_all($src, TOKEN_PARSE);
        return ["ok"];
    } catch (Throwable $e) {
        return [$e instanceof ParseError ? "error" : "other", $e->getLine(),
            $e instanceof ParseError ? $e->getMessage() : get_class($e) . ": " . $e->getMessage(),
            $e->getFile(), $call];
    }
}
function parse_second($src) {
    try {
        $call = __LINE__ + 1;
        token_get_all($src, TOKEN_PARSE);
        return ["ok"];
    } catch (Throwable $e) {
        return [$e instanceof ParseError ? "error" : "other", $e->getLine(),
            $e instanceof ParseError ? $e->getMessage() : get_class($e) . ": " . $e->getMessage(),
            $e->getFile(), $call];
    }
}
foreach (glob('/corpus/*.php') as $f) {
    $src = file_get_contents($f);
    $a = parse_first($src);
    $b = parse_second($src);
    if ($a[0] !== $b[0] || ($a[0] !== "ok" && ($a[2] !== $b[2] || $a[3] !== $b[3]))) {
        throw new RuntimeException("Inconsistent parse observations: " . basename($f));
    }
    if ($a[0] === "ok") {
        echo json_encode([basename($f), "ok"], JSON_INVALID_UTF8_SUBSTITUTE), "\n";
        continue;
    }
    $caller = $a[3] === __FILE__ && $a[1] === $a[4] && $b[1] === $b[4];
    if (!$caller && $a[1] !== $b[1]) {
        throw new RuntimeException("Unresolved parse location: " . basename($f));
    }
    echo json_encode([basename($f), $a[0], $a[1], $a[2], [
        "kind" => $caller ? "caller" : "source",
        "call_lines" => [$a[4], $b[4]],
        "observed" => [[$a[3], $a[1]], [$b[3], $b[1]]],
    ]], JSON_INVALID_UTF8_SUBSTITUTE), "\n";
}
