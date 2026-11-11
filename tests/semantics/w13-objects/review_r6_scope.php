<?php
// REQ-018/020; T-035/036/039/040. Access uses lexical scope; diagnostics use called scope.
class W13R6ScopeTarget { public private(set) int $p = 1; }
class W13R6ScopeTargetReadonly { public private(set) readonly int $p; }
class W13R6ScopeOther {
    public function fail($target): void { $target->p = 2; }
}
class W13R6ScopeString {
    public function __toString(): string { throw new Exception('convert'); }
}
function w13R6ScopeGlobal($target): void { $target->p = 2; }
class W13R6ScopeParent {
    public int $v { set { $this->p = $value; } }
    public function fail(): void { $this->p = 2; }
    public function reference(): void { $r =& $this->p; }
    public static function staticFail($target): void { $target->p = 2; }
    public function other($target): void { $target->p = 2; }
    public function otherUnset($target): void { unset($target->p); }
    public function nested($target): array {
        $first = w13R6Attempt(fn() => (new W13R6ScopeOther)->fail($target));
        return [$first, w13R6Attempt(fn() => $this->fail())];
    }
    public function globalCall($target): array {
        $first = w13R6Attempt(fn() => w13R6ScopeGlobal($target));
        return [$first, w13R6Attempt(fn() => $this->fail())];
    }
    public function conversion(): array {
        $first = w13R6Attempt(fn() => (string) new W13R6ScopeString);
        return [$first, w13R6Attempt(fn() => $this->fail())];
    }
}
class W13R6ScopeChild extends W13R6ScopeParent { public private(set) int $p = 1; }
$tests['r6-scope-write'] = fn() => w13R6Attempt(fn() => (new W13R6ScopeChild)->fail());
$tests['r6-scope-reference'] = fn() => w13R6Attempt(fn() => (new W13R6ScopeChild)->reference());
$tests['r6-scope-static'] = fn() => w13R6Attempt(fn() => W13R6ScopeChild::staticFail(new W13R6ScopeTarget));
$tests['r6-scope-target'] = fn() => w13R6Attempt(fn() => (new W13R6ScopeChild)->other(new W13R6ScopeTarget));
$tests['r6-scope-readonly-private'] = fn() => w13R6Attempt(fn() => (new W13R6ScopeChild)->other(new W13R6ScopeTargetReadonly));
$tests['r6-scope-readonly-private-unset'] = fn() => w13R6Attempt(fn() => (new W13R6ScopeChild)->otherUnset(new W13R6ScopeTargetReadonly));
$tests['r6-scope-nested'] = fn() => (new W13R6ScopeChild)->nested(new W13R6ScopeTarget);
$tests['r6-scope-global'] = fn() => (new W13R6ScopeChild)->globalCall(new W13R6ScopeTarget);
$tests['r6-scope-conversion'] = fn() => (new W13R6ScopeChild)->conversion();
$tests['r6-scope-hook'] = function() { $o = new W13R6ScopeChild; return w13R6Attempt(function() use($o) { $o->v = 2; }); };
