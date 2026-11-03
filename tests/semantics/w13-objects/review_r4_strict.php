<?php
declare(strict_types=1);
// REQ-012/018; T-023/024/035/036. Actual strict caller and return definitions.
function w13R4StrictAccept($sink, $value): string {
    return $sink->acceptStrict($value);
}
class W13R4StrictCallableSink extends W13R4CallableSink {
    public function returnStrict(): callable { return $GLOBALS['w13r4_callable']; }
    public function badReturnStrict(): callable { return $GLOBALS['w13r4_bad_callable']; }
}
