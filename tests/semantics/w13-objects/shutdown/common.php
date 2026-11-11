<?php
// REQ-020; T-039/040. Ordinary object-store slots, with no builtin objects.
class W13ShutdownObject {
    public function __construct(public string $name){}
    public function __destruct(){
        echo $this->name,"\n";
        if($this->name==='maker'){
            $GLOBALS['new1']=new self('new');$GLOBALS['new2']=$GLOBALS['new1'];
        }
    }
}
$hole=new W13ShutdownObject('hole');
$old=new W13ShutdownObject('old');$old2=$old;
if(!$lateHole)unset($hole);
$maker=new W13ShutdownObject('maker');$maker2=$maker;
if($lateHole)unset($hole);
echo "body-end\n";
