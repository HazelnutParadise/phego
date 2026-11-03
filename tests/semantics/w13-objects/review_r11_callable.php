<?php
// REQ-018; T-035/036. Relative callables use the callee frame and warn once.
$tests ??= [];
class W13R11P {
    static function stat() {}
    function inst() {}
    function arg(callable $c) { return 'body'; }
    static function sarg(callable $c) { return 'body'; }
    function ref(callable &$c) { return 'body'; }
    static function sref(callable &$c) { return 'body'; }
    function ret($c): callable { return $c; }
    static function sret($c): callable { return $c; }
    function unionString($c): callable|string { return $c; }
    function unionArray($c): callable|array { return $c; }
}
class W13R11C extends W13R11P { static function childOnly() {} }
class W13R11O extends W13R11P {
    function arg(callable $c) { return 'body'; }
    function ref(callable &$c) { return 'body'; }
    function ret($c): callable { return $c; }
}
class W13R11Storage { public string|array $slot; }
$w13R11CallableInputs = ['self::stat','parent::stat','static::childOnly','self::inst','self::missing','\\self::stat','SELF::stat','W13R11P::inst','W13R11C::inst','self::childOnly','parent::inst'];
$w13R11CallableModes = ['P-arg','C-arg','O-arg','P-sarg','C-sarg','P-ret','C-ret','O-ret','P-sret','C-sret','P-ref','C-ref','O-ref','P-sref','C-sref','P-union','unscoped'];
$w13R11CallableNames = [];
foreach ($w13R11CallableInputs as $i=>$input) foreach (['string','array'] as $shape) foreach ($w13R11CallableModes as $mode) {
    $name='r11-callable-'.$i.'-'.$shape.'-'.$mode;
    $w13R11CallableNames[]=$name;
    $tests[$name]=function()use($input,$shape,$mode) {
        $value=$shape==='string' ? $input : explode('::',$input,2);
        $storage=new W13R11Storage; $storage->slot=$value;
        $object=match($mode[0]) {'C'=>new W13R11C,'O'=>new W13R11O,default=>new W13R11P};
        try {
            if($mode==='unscoped') return [is_callable($value)];
            if(str_ends_with($mode,'-union')) { $shape==='string' ? $object->unionString($value) : $object->unionArray($value); }
            elseif(str_ends_with($mode,'-sarg')) { $object::sarg($value); }
            elseif(str_ends_with($mode,'-sret')) { $object::sret($value); }
            elseif(str_ends_with($mode,'-sref')) { $object::sref($storage->slot); }
            elseif(str_ends_with($mode,'-arg')) { $object->arg($value); }
            elseif(str_ends_with($mode,'-ref')) { $object->ref($storage->slot); }
            else { $object->ret($value); }
            return ['ok'];
        } catch(Throwable $e) { return [$e::class,$e->getMessage()]; }
    };
}
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '')===__FILE__) {
    $cases=[];
    foreach($w13R11CallableNames as $name) {
        $diagnostics=[];
        set_error_handler(function($level,$message)use(&$diagnostics){$diagnostics[]=[$level,$message];return true;});
        $result=$tests[$name](); restore_error_handler();
        $cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];
    }
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
