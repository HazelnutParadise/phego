<?php
// Exact value encoding shared by the W-10 reference scripts: floats by their
// IEEE-754 bits, strings by hex.
function enc($v) {
    switch (gettype($v)) {
    case 'NULL': return 'N';
    case 'boolean': return $v ? 'b:1' : 'b:0';
    case 'integer': return 'i:' . $v;
    case 'double': return 'd:' . bin2hex(pack('E', $v));
    case 'string': return 's:' . bin2hex($v);
    case 'array':
        $out = [];
        foreach ($v as $k => $e) { $out[] = [enc($k), enc($e)]; }
        return 'a:' . json_encode($out);
    }
    throw new LogicException('unencodable ' . gettype($v));
}
