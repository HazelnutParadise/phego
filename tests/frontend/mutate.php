<?php
// Deterministic token-level mutants of each corpus file: delete, duplicate,
// or insert a token taken from elsewhere in the same file.
mt_srand(20261031);
$n = 0;
$text = fn($t) => is_array($t) ? $t[1] : $t;
foreach (glob('/corpus/*.php') as $f) {
    $toks = @token_get_all(file_get_contents($f));
    $sig = [];
    foreach ($toks as $i => $t) {
        if (!is_array($t) || !in_array($t[0], [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT, T_INLINE_HTML, T_OPEN_TAG], true)) {
            $sig[] = $i;
        }
    }
    if (count($sig) < 4) continue;
    for ($k = 0; $k < 3; $k++) {
        $copy = array_map($text, $toks);
        $i = $sig[mt_rand(0, count($sig) - 1)];
        $j = $sig[mt_rand(0, count($sig) - 1)];
        $op = mt_rand(0, 2);
        if ($op === 0) $copy[$i] = '';
        elseif ($op === 1) $copy[$i] = $copy[$i] . ' ' . $copy[$i];
        else $copy[$i] = $copy[$i] . ' ' . $text($toks[$j]);
        file_put_contents(sprintf('/out/%016x.php', $n++), implode('', $copy));
    }
}
echo $n, "\n";
