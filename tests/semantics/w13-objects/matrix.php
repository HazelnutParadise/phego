<?php
// Original fixtures: REQ-012/018/019/020/021; T-023/024/035-042.
declare(strict_types=1);
function attempt(callable $f): mixed {
    try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; }
}
class W13Base {
    public int $i;
    public ?int $nullable = null;
    public readonly int $ro;
    private string $secret = 'base';
    protected string $protected = 'protected';
    public function initialize(int $v): void { $this->ro = $v; }
    public function baseSecret(): string { return $this->secret; }
}
class W13Child extends W13Base {
    private string $secret = 'child';
    public function childSecret(): string { return $this->secret; }
}
#[AllowDynamicProperties] class W13Dynamic {}
class W13Hooks {
    public int $backed = 1 {
        get { $GLOBALS['events'][] = 'get'; return $this->backed; }
        set { $GLOBALS['events'][] = 'set'; $this->backed = $value * 2; }
    }
    public int $virtual { get => $this->backed + 10; }
}
class W13Lazy { public int $i; public string $s = 'default'; }
enum W13Enum: string { case A = 'a'; case B = 'b'; }
$tests = [
    'uninitialized' => function() { $o = new W13Base; return attempt(fn() => $o->i); },
    'typed-write' => function() { $o = new W13Base; return attempt(function() use ($o) { $o->i = 'x'; }); },
    'typed-null' => function() { $o = new W13Base; return attempt(function() use ($o) { $o->i = null; }); },
    'typed-reference' => function() { $o = new W13Base; $o->i = 1; $r =& $o->i; $e = attempt(function() use (&$r) { $r = 'x'; }); return [$e, $r, $o->i]; },
    'reference-detach' => function() { $o = new W13Base; $o->i = 1; $r =& $o->i; unset($o->i); $r = 'free'; return [$r, isset($o->i)]; },
    'reference-destroy' => function() { $o = new W13Base; $o->i = 1; $r =& $o->i; unset($o); $r = 'free'; return $r; },
    'reference-array-promotion' => function() { $o = new W13Base; $r =& $o->nullable; return attempt(function() use (&$r) { $r[] = 1; }); },
    'readonly-init' => function() { $o = new W13Base; $e = attempt(function() use ($o) { $o->ro = 1; }); return $e; },
    'readonly-rewrite' => function() { $o = new W13Base; $o->initialize(1); return attempt(function() use ($o) { $o->ro = 2; }); },
    'readonly-reference' => function() { $o = new W13Base; $o->initialize(1); return attempt(function() use ($o) { $r =& $o->ro; }); },
    'readonly-unset' => function() { $o = new W13Base; $o->initialize(1); return attempt(function() use ($o) { unset($o->ro); }); },
    'private-access' => function() { $o = new W13Child; return attempt(fn() => $o->secret); },
    'protected-access' => function() { $o = new W13Child; return attempt(fn() => $o->protected); },
    'private-inheritance' => function() { $o = new W13Child; return [$o->baseSecret(), $o->childSecret(), array_keys((array)$o)]; },
    'dynamic-order' => function() { $o = new W13Dynamic; $o->a=1; $o->b=2; unset($o->a); $o->a=3; return (array)$o; },
    'declared-order' => function() { $o = new W13Base; $o->i=1; unset($o->nullable); $o->nullable=2; return array_keys((array)$o); },
    'dynamic-deprecation' => function() { $o = new W13Base; $o->extra=1; $o->extra=2; return $o->extra; },
    'clone-reference' => function() { $o = new W13Base; $o->i=1; $r =& $o->i; $c=clone $o; $r=2; return [$o->i,$c->i,$o===$c,$o==$c]; },
    'clone-separate' => function() { $o = new W13Base; $o->i=1; $c=clone $o; $c->i=2; return [$o->i,$c->i,$o==$c]; },
    'hooks' => function() { $GLOBALS['events']=[]; $o=new W13Hooks; $o->backed=3; $a=$o->backed; $b=$o->virtual; return [$a,$b,$GLOBALS['events']]; },
    'virtual-write' => function() { $o=new W13Hooks; return attempt(function() use($o) {$o->virtual=1;}); },
    'virtual-reference' => function() { $o=new W13Hooks; return attempt(function() use($o) {$r =& $o->virtual;}); },
    'hook-reflection' => function() { $r=new ReflectionClass(W13Hooks::class); $a=[]; foreach($r->getProperties() as $p) {$a[]=[$p->getName(),$p->isVirtual(),$p->hasHook(PropertyHookType::Get),$p->hasHook(PropertyHookType::Set)];} return $a; },
    'lazy-ghost' => function() { $calls=0; $r=new ReflectionClass(W13Lazy::class); $o=$r->newLazyGhost(function($o) use(&$calls) {$calls++;$o->i=7;}); $before=[$r->isUninitializedLazyObject($o),$calls]; $i=$o->i; return [$before,$i,$o->s,$calls,$r->isUninitializedLazyObject($o)]; },
    'lazy-failure-retry' => function() { $calls=0; $r=new ReflectionClass(W13Lazy::class); $o=$r->newLazyGhost(function($o) use(&$calls) {$o->i=++$calls; if($calls===1) throw new Exception('retry');}); $e=attempt(fn()=>$o->i); $state=$r->isUninitializedLazyObject($o); return [$e,$state,$o->i,$calls]; },
    'lazy-proxy' => function() { $calls=0; $r=new ReflectionClass(W13Lazy::class); $real=new W13Lazy; $real->i=9; $o=$r->newLazyProxy(function() use(&$calls,$real) {$calls++;return $real;}); return [$o===$real,$o->i,$calls,$r->getLazyInitializer($o)===null]; },
    'enum-identity' => function() { return [W13Enum::A===W13Enum::A,W13Enum::A===W13Enum::B,W13Enum::A->name,W13Enum::A->value,W13Enum::A==W13Enum::B,W13Enum::A<=>W13Enum::B]; },
    'enum-clone' => function() { return attempt(fn()=>clone W13Enum::A); },
    'weak-reference' => function() { $o=new stdClass; $w=WeakReference::create($o); $same=$w===WeakReference::create($o); unset($o); return [$same,$w->get()]; },
    'weak-map-backedge' => function() { $m=new WeakMap; $o=new stdClass; $w=WeakReference::create($o); $m[$o]=$o; unset($o); $count=gc_collect_cycles(); return [$count,count($m),$w->get()===null]; },
    'cycle' => function() { $o=new stdClass; $o->self=$o; $w=WeakReference::create($o); unset($o); $n=gc_collect_cycles(); return [$n,$w->get()===null]; },
];
$cases=[];
require __DIR__.'/gc.php';
require __DIR__.'/methods.php';
require __DIR__.'/resources.php';
require __DIR__.'/gc_corpus.php';
require __DIR__.'/weakmaps.php';
require __DIR__.'/properties.php';
require __DIR__.'/types.php';
require __DIR__.'/method_ownership.php';
require __DIR__.'/names.php';
require __DIR__.'/proxy.php';
require __DIR__.'/captures.php';
require __DIR__.'/hook_frames.php';
require __DIR__.'/statics.php';
require __DIR__.'/review.php';
require __DIR__.'/review_r2_returns.php';
require __DIR__.'/review_r2_values.php';
require __DIR__.'/review_r2_properties.php';
require __DIR__.'/review_r2_lazy.php';
require __DIR__.'/review_r2_cleanup.php';
require __DIR__.'/review_r4_weakmap.php';
require __DIR__.'/review_r4_access.php';
require __DIR__.'/review_r5_references.php';
require __DIR__.'/review_r5_lifetime.php';
require __DIR__.'/review_r5_operations.php';
require __DIR__.'/review_r6_references.php';
require __DIR__.'/review_r6_lazy.php';
require __DIR__.'/review_r6_scope.php';
require __DIR__.'/review_r6_raw_unset.php';
require __DIR__.'/review_r6_proxy_guard.php';
require __DIR__.'/review_r6_guard_lifetime.php';
require __DIR__.'/review_r7_lazy_state.php';
require __DIR__.'/review_r7_defaults.php';
require __DIR__.'/review_r8_rollback.php';
require __DIR__.'/review_r9_churn.php';
require __DIR__.'/review_r10_proxy.php';
require __DIR__.'/review_r10_proxy_hooks.php';
require __DIR__.'/review_r10_proxy_static.php';
require __DIR__.'/review_r10_lifetime.php';
require __DIR__.'/review_r10_alias.php';
require __DIR__.'/review_r11_enum.php';
require __DIR__.'/review_r12_magic.php';
require __DIR__.'/review_r12_references.php';
require __DIR__.'/review_r12_setters.php';
require __DIR__.'/review_r12_setter_holds.php';
require __DIR__.'/review_r13_graph.php';
require __DIR__.'/review_r11_traits.php';
require __DIR__.'/review_r11_trait_inheritance.php';
require __DIR__.'/review_r11_callable.php';
require __DIR__.'/review_r3_enum.php';
require __DIR__.'/review_r3_properties.php';
require __DIR__.'/review_r3_values.php';
require __DIR__.'/review_r3_hooks.php';
require __DIR__.'/review_r3_frames.php';
require __DIR__.'/review_r3_typed_return.php';
require __DIR__.'/review_r3_callable_return.php';
foreach($tests as $name=>$test) {
    // These cases have exact independent observations. Their consecutive
    // execution corrupts pinned PHP; preserve that separate diagnostic.
    if(in_array($name,['lazy-rollback-write-declared','lazy-rollback-write-dynamic'],true))continue;
    gc_collect_cycles();
    $diagnostics=[];
    set_error_handler(function($level,$message) use(&$diagnostics) {$diagnostics[]=[$level,$message];return true;});
    $result=attempt($test);
    restore_error_handler();
    $cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];
}
echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_THROW_ON_ERROR|JSON_PRETTY_PRINT),"\n";
