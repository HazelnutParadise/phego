<?php
// Original MIT fixtures. This runs only in the pinned disposable reference.
// REQ-016/024/034/054; T-031/032/047/048/067/068/107/108.
function w15enc($v) {
    if ($v === null) return 'N';
    if (is_bool($v)) return 'b:'.(int)$v;
    if (is_int($v)) return 'i:'.$v;
    if (is_float($v)) return 'd:'.bin2hex(pack('E', $v));
    if (is_string($v)) return 's:'.bin2hex($v);
    if (is_array($v)) {
        $out = [];
        foreach ($v as $k => $x) $out[] = [w15enc($k), w15enc($x)];
        return 'a:'.json_encode($out);
    }
    throw new LogicException('unsupported fixture return');
}
function w15mark($s) { echo 'C:'.$s."\n"; return 5; }
class W15Probe {
    public function __construct(private string $name, private bool $fail = false) {}
    public function __destruct() {
        echo 'D:'.$this->name."\n";
        if ($this->fail) throw new RuntimeException('destruct:'.$this->name);
    }
}
$fixtures = [
    'nested' => 'return (1+2)*(3+4);',
    'width' => 'return 2147483647+1;',
    'overflow' => 'return 9223372036854775807+1;',
    'bitwise' => 'return (12|3) ^ (9&7);',
    'bool' => 'return !false;',
    'strict' => 'return (1===true) !== (2===2);',
    'bytes' => 'return ("a\x00"."\xff")."b";',
    'alias' => '$a=1; $b=&$a; $b=1+2; return $a;',
    'rebind' => '$a=1; $b=&$a; unset($b); $b=7; return $a;',
    'cow_reference' => '$a=1; $x=[&$a]; $y=$x; $y[0]=2+3; return $x[0];',
    'dynamic' => '$n="a"; $a=1; $$n=2+3; return $a;',
    'extract' => '$a=1; extract(["a"=>2+3]); return $a;',
    'compact' => '$a=1+2; return compact("a");',
    'destructor' => '$a=new W15Probe("a"); $a=1+2; return 8;',
    'destructor_throw' => '$a=new W15Probe("a",true); $a=1+2; return w15mark("bad");',
    'callback' => '$a=w15mark("a"); return (1+2)+w15mark("b");',
    'lazy_and' => '$a=false; return $a && w15mark("bad");',
    'lazy_or' => '$a=true; return $a || w15mark("bad");',
    'lazy_ternary' => '$a=true; return $a ? (1+2) : (1/0);',
    'lazy_coalesce' => '$a=7; return $a ?? w15mark("bad");',
    'defined' => '$a=1; return !isset($a);',
    'specialized_int' => '$a="12"; $b="3"; return ((int)$a)|((int)$b);',
    'zero_power' => '$a=0; return $a ** -1;',
    'nan_bool' => '$a=NAN; return !$a;',
    'nan_string' => '$a=NAN; return $a."x";',
    'numeric_warning' => 'return "12tail" + (1+2);',
    'division' => 'return (1+2)/0;',
    'warning_before_callback' => '$a="12tail"+1; return w15mark("after");',
    'if' => '$a=true; if($a){return 1+2;} else {return 1/0;}',
];
// The bounded LCG uses only arithmetic exactly representable in both profiles.
// Freeze this seed before implementing passes; every case is independently run.
$seed = 15031;
for ($i=0; $i<256; $i++) {
    $seed=($seed*17+43)%65521; $a=$seed-32760;
    $seed=($seed*17+43)%65521; $b=$seed-32760;
    $op=['+','-','*','&','|','^'][$i%6];
    $fixtures['random_'.$i]='return (('.$a.')'.$op.'('.$b.'))+(2*3);';
}
$cases=[];
foreach ($fixtures as $name=>$source) {
    $diagnostics=[]; $result=''; $error=[];
    set_error_handler(function($level,$message,$file,$line) use (&$diagnostics) {
        $diagnostics[]=[$level,$message,$line]; return true;
    });
    ob_start();
    try { $result=w15enc((eval('return function(){'.$source.'};'))()); }
    catch (Throwable $e) { $error=[get_class($e),$e->getMessage()]; }
    $out=ob_get_clean();
    restore_error_handler();
    $cases[]=['name'=>$name,'source'=>$source,'result'=>$result,'error'=>$error,'diagnostics'=>$diagnostics,'output'=>$out];
}
echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'seed'=>15031,'cases'=>$cases], JSON_UNESCAPED_SLASHES|JSON_THROW_ON_ERROR),"\n";
