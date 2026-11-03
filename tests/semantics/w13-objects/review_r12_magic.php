<?php
// REQ-012/018; T-023/024/035/036. Effective magic handlers ignore handler visibility.
$tests ??= [];
// Declaration diagnostics are preserved by the standalone probe. Frontend
// declaration checking is separate from the runtime cases below.
$w13R12MagicDeclarations=[];
set_error_handler(function($l,$m)use(&$w13R12MagicDeclarations){$w13R12MagicDeclarations[]=[$l,$m];return true;});
foreach(['public','protected','private']as$visibility) {
    $class='W13R12Magic_'.$visibility;
    eval('class '.$class.' {'.$visibility.' function __call($name,$args){return ["magic:".$name,$args];} private function secret(){} protected function guarded(){} '.$visibility.' static function __callStatic($name,$args){return ["static:".$name,$args];}}');
}
eval('class W13R12MagicParent {private function __call($name,$args){return "parent:".$name;} function forward($target){return $target->missing();} function explicit($target){return $target->__call("named",[]);}}');
eval('class W13R12MagicChild extends W13R12MagicParent {protected function __call($name,$args){return "child:".$name;}}');
restore_error_handler();
$w13R12MagicNames=[];
foreach(['public','protected','private']as$visibility) {
    $class='W13R12Magic_'.$visibility;
    foreach(['missing','secret','guarded','explicit','callable-object','callable-class']as$mode) {
        $name='r12-magic-'.$visibility.'-'.$mode;$w13R12MagicNames[]=$name;
        $tests[$name]=function()use($class,$mode) {
            $o=new$class;
            if($mode==='callable-object')return is_callable([$o,'missing']);
            if($mode==='callable-class')return is_callable([$class,'missing']);
            if($mode==='explicit')return$o->__call('named',[]);
            return$o->$mode(7,tail:8);
        };
    }
}
foreach(['forward','explicit','global','callable']as$mode) {
    $name='r12-magic-child-'.$mode;$w13R12MagicNames[]=$name;
    $tests[$name]=function()use($mode) {
        $o=new W13R12MagicChild;
        return match($mode){'forward'=>$o->forward($o),'explicit'=>$o->explicit($o),'global'=>$o->missing(),default=>is_callable([$o,'missing'])};
    };
}
if(realpath($_SERVER['SCRIPT_FILENAME']??'')===__FILE__) {
    $cases=[];foreach($w13R12MagicNames as$name){$diagnostics=[];set_error_handler(function($l,$m)use(&$diagnostics){$diagnostics[]=[$l,$m];return true;});$result=$tests[$name]();restore_error_handler();$cases[$name]=['result'=>$result,'diagnostics'=>$diagnostics];}
    echo json_encode(['php'=>PHP_VERSION,'int_size'=>PHP_INT_SIZE,'declaration_diagnostics'=>$w13R12MagicDeclarations,'cases'=>$cases],JSON_PRETTY_PRINT|JSON_THROW_ON_ERROR),"\n";
}
