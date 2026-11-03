<?php
// REQ-018/020; T-035/036/039/040. Shared interface ancestors are one DAG.
$tests ??= [];
interface W13R13Root {}
interface W13R13Unrelated {}
interface W13R13Extra {}
interface W13R13Left0 extends W13R13Root {}
interface W13R13Right0 extends W13R13Root {}
for($i=1;$i<=12;$i++) {
    $previous=$i-1;
    eval('interface W13R13Left'.$i.' extends W13R13Left'.$previous.', W13R13Right'.$previous.' {}');
    eval('interface W13R13Right'.$i.' extends W13R13Left'.$previous.', W13R13Right'.$previous.' {}');
}
class W13R13Parent implements W13R13Left12,W13R13Right12 {}
class W13R13Child extends W13R13Parent implements W13R13Extra {
    function __toString():string {return 'child';}
}
$w13R13Targets=['root'=>'W13R13Root','left0'=>'W13R13Left0','right0'=>'W13R13Right0','left12'=>'W13R13Left12','right12'=>'W13R13Right12','parent'=>'W13R13Parent','child'=>'W13R13Child','extra'=>'W13R13Extra','stringable'=>'Stringable','unrelated'=>'W13R13Unrelated','missing'=>'W13R13Missing','case'=>'w13r13root','qualified'=>'\\W13R13Root'];
$w13R13GraphNames=[];
foreach(['parent'=>W13R13Parent::class,'child'=>W13R13Child::class]as$kind=>$class)foreach($w13R13Targets as$label=>$target) {
    $name='r13-graph-'.$kind.'-'.$label;$w13R13GraphNames[]=$name;
    $tests[$name]=fn()=>is_a(new$class,$target);
}
foreach([40,80]as$depth) {
    $prefix='W13R13Deep'.$depth;
    eval('interface '.$prefix.'Root {} interface '.$prefix.'Missing {}');
    eval('interface '.$prefix.'Left0 extends '.$prefix.'Root {} interface '.$prefix.'Right0 extends '.$prefix.'Root {}');
    for($i=1;$i<=$depth;$i++) {
        $previous=$i-1;
        eval('interface '.$prefix.'Left'.$i.' extends '.$prefix.'Left'.$previous.', '.$prefix.'Right'.$previous.' {}');
        eval('interface '.$prefix.'Right'.$i.' extends '.$prefix.'Left'.$previous.', '.$prefix.'Right'.$previous.' {}');
    }
    $class=$prefix.'Receiver';
    eval('class '.$class.' implements '.$prefix.'Left'.$depth.', '.$prefix.'Right'.$depth.' {}');
    foreach(['root'=>$prefix.'Root','missing'=>$prefix.'Missing']as$label=>$target) {
        $name='r13-deep-'.$depth.'-'.$label;$w13R13GraphNames[]=$name;
        $tests[$name]=fn()=>is_a(new$class,$target);
    }
}
if(realpath($_SERVER['SCRIPT_FILENAME']??'')===__FILE__) {
    $cases=[];foreach($w13R13GraphNames as$name)$cases[$name]=['result'=>$tests[$name](),'diagnostics'=>[]];
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
