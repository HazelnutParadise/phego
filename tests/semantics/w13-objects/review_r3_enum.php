<?php
// REQ-018; T-035/036. Empty enum descriptors retain their intrinsic fields.
enum W13R3EmptyUnit {}
enum W13R3EmptyInt:int {}
enum W13R3EmptyString:string {}
foreach(['unit'=>W13R3EmptyUnit::class,'int'=>W13R3EmptyInt::class,'string'=>W13R3EmptyString::class] as $kind=>$class){
    $tests['r3-enum-empty-'.$kind]=function()use($class){
        $r=new ReflectionClass($class);$props=[];
        foreach($r->getProperties() as $p)$props[]=[$p->name,(string)$p->getType(),$p->isReadOnly()];
        return [attempt(fn()=>new $class),$props];
    };
}
