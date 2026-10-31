<?php
// Emit one JSON line per file: [name, "ok"] or [name, "error", line, message].
foreach (glob('/corpus/*.php') as $f) {
    $src = file_get_contents($f);
    try {
        token_get_all($src, TOKEN_PARSE);
        echo json_encode([basename($f), "ok"], JSON_INVALID_UTF8_SUBSTITUTE), "\n";
    } catch (ParseError $e) {
        echo json_encode([basename($f), "error", $e->getLine(), $e->getMessage()], JSON_INVALID_UTF8_SUBSTITUTE), "\n";
    } catch (Throwable $e) {
        echo json_encode([basename($f), "other", $e->getLine(), get_class($e) . ": " . $e->getMessage()], JSON_INVALID_UTF8_SUBSTITUTE), "\n";
    }
}
