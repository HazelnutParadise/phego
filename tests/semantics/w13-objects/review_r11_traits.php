<?php
// REQ-018; T-035/036. Compatible trait declarations share one consuming-class slot.
$tests ??= [];
if (!function_exists('w13R11Attempt')) {
    function w13R11Attempt(callable $f): mixed { try { return $f(); } catch (Throwable $e) { return [$e::class, $e->getMessage()]; } }
}
$w13R11TraitFamilies = ['private-int','private-null','private-uninitialized','private-nullable','private-union','private-array','private-empty-array','private-string','private-float','private-self','public-int','protected-int','ancestor-private','class-own-private','nested-private','readonly-private'];
$w13R11TraitClasses = [];
foreach ($w13R11TraitFamilies as $family) {
    $suffix = str_replace('-', '_', $family); $class = 'W13R11Traits_'.$suffix;
    $a = 'W13R11TraitA_'.$suffix; $b = 'W13R11TraitB_'.$suffix; $parent = 'W13R11TraitParent_'.$suffix;
    $visibility = str_starts_with($family, 'public') ? 'public' : (str_starts_with($family, 'protected') ? 'protected' : 'private');
    [$typeA,$typeB,$defaultA,$defaultB] = match ($family) {
        'private-null' => ['', '', '', '=null'],
        'private-uninitialized', 'readonly-private' => ['int', 'int', '', ''],
        'private-nullable' => ['?int', 'int|null', '=null', '=null'],
        'private-union' => ['int|string', 'string|int', '=1', '=1'],
        'private-array' => ['array', 'array', "=['k'=>1,2=>'v']", "=['k'=>1,2=>'v']"],
        'private-empty-array' => ['array', 'array', '=[]', '=[]'],
        'private-string' => ['string', 'string', "=''", "=''"],
        'private-float' => ['float', 'float', '=1', '=1.0'],
        'private-self' => ['self', $class, '', ''],
        default => ['int', 'int', '=1', '=1'],
    };
    $readonly = $family === 'readonly-private' ? 'readonly ' : '';
    eval('trait '.$a.' {'.$visibility.' '.$readonly.$typeA.' $x '.$defaultA.';}');
    if ($family === 'nested-private') eval('trait '.$b.' {use '.$a.';}');
    else eval('trait '.$b.' {'.$visibility.' '.$readonly.$typeB.' $x '.$defaultB.';}');
    $extends = '';
    if ($family === 'ancestor-private') {
        eval('class '.$parent.' {private int $x=9; public function parentPeek():mixed{return $this->x;}}');
        $extends = ' extends '.$parent;
    }
    $own = $family === 'class-own-private' ? 'private int $x=1;' : '';
    eval('class '.$class.$extends.' {use '.$a.','.$b.';'.$own.' public function peek():mixed{return $this->x;}}');
    $w13R11TraitClasses[$family] = $class;
}
$w13R11TraitNames = [];
foreach ($w13R11TraitFamilies as $family) foreach (['normal','lazy'] as $mode) {
    $name = 'r11-trait-'.$family.'-'.$mode; $w13R11TraitNames[] = $name;
    $tests[$name] = function() use($family,$mode,$w13R11TraitClasses) {
        $class = $w13R11TraitClasses[$family]; $reflection = new ReflectionClass($class); $calls=0;
        $object = $mode === 'lazy' ? $reflection->newLazyGhost(function()use(&$calls){++$calls;}) : new $class;
        $metadata=[]; foreach($reflection->getProperties() as $property) $metadata[]=[$property->getName(),$property->getDeclaringClass()->getName()];
        $peek = function() use($object) { $value=$object->peek(); return [get_debug_type($value),$value]; };
        $initial = $mode === 'normal' ? w13R11Attempt($peek) : null;
        $before = [$reflection->isUninitializedLazyObject($object),$calls];
        $rhs = match($family) {'private-array','private-empty-array'=>[2], 'private-string','private-union'=>'new', 'private-float'=>7.5, 'private-self'=>$object, default=>7};
        $write = w13R11Attempt(fn()=>$reflection->getProperty('x')->setRawValueWithoutLazyInitialization($object,$rhs));
        $afterRaw = [$reflection->isUninitializedLazyObject($object),$calls];
        $seen = $family === 'private-self' ? w13R11Attempt(fn()=>$object->peek()===$object) : w13R11Attempt($peek);
        $parent = $family === 'ancestor-private' ? w13R11Attempt(fn()=>$object->parentPeek()) : null;
        $out=[$metadata,$initial,$before,$write,$afterRaw,$seen,$parent,[$reflection->isUninitializedLazyObject($object),$calls]];
        return $out;
    };
}
if (realpath($_SERVER['SCRIPT_FILENAME'] ?? '') === __FILE__) {
    $cases=[];
    foreach($w13R11TraitNames as $name){
        gc_collect_cycles();$diagnostics=[];
        set_error_handler(function($level,$message)use(&$diagnostics){$diagnostics[]=[$level,$message];return true;});
        $result=w13R11Attempt($tests[$name]);restore_error_handler();$cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];
    }
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
