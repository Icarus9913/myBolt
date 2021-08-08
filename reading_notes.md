## 笔记

<br/>

##### 文件组织结构:
- bucket.go：对 bucket 操作的高层封装。包括kv 的增删改查、子bucket 的增删改查以及 B+ 树拆分和合并。
- node.go：对 node 所存元素和 node 间关系的相关操作。节点内所存元素的增删、加载和落盘，访问孩子兄弟元素、拆分与合并的详细逻辑。
- cursor.go：实现了类似迭代器的功能，可以在 B+ 树上的叶子节点上进行随意游走。

<br/>

##### 读写流程
在打开一个已经存在的 db 时，会首先将 db 文件映射到内存空间，然后解析元信息页，最后加载空闲列表。
  
在 db 进行读取时，会按需将访问路径上的 page 加载到内存，并转换为 node，进行缓存。
  
在 db 进行修改时，使用 COW (copy-on-write)原则，所有修改不在原地，而是在改动前先复制一份。
如果叶子节点 node 需要修改，则 root bucket 到该 node 路径上所涉及的所有节点都需要修改。
这些节点都需要新申请空间，然后持久化，这些和事务的实现息息相关，之后会在本系列事务文章中做详细说明。

<br/>

##### 数据结构
boltdb 在数据组织方面只使用了两个概念：页（page） 和节点 （node）。
每个数据库对应一个文件，每个文件中包含一系列线性组织的页。页的大小固定，依其性质不同，分为四种类型：元信息页、空闲列表页、叶子节点页、中间节点页。
打开数据库时，会渐次进行以下操作：

1. 利用 mmap 将数据库文件映射到内存空间。
2. 解析元信息页，获取空闲列表页 id 和 root bucket 页 id。
3. 依据空闲列表页 id ，将所有空闲页列表载入内存。
4. 依据 root bucket 起始页地址，解析 root bucket 根节点。
5. 根据读写需求，从树根开始遍历，按需将访问路径上的数据页（中间节点页和叶子节点页）载入内存成为节点（node）。
  
可以看出，节点分两种类型：中间节点（branch node）和叶子节点（leaf node）。  
另外需要注意的是，bucket 可以进行无限嵌套，导致这一块稍微有点不好理解。在下一篇 boltdb 的索引设计中，将详细剖析 boltdb 是如何组织多个 bucket 以及单个 bucket 内的 B+ 树索引的。


BoltDB在逻辑上以桶来组织数据,一个桶可以看做一个命名空间,是一组KV对的集合,和对象存储的桶概念类似.
每个桶对应一棵B+树,命名空间是可以嵌套的,因此BoltDB的Bucket间也是允许嵌套的.
在实现上来说,子bucket的root node的page id保存在父bucket叶子节点上实现嵌套.

每个db文件,是一组树形组织的B+树. 分支节点用于查找,叶子节点存数据.
1. 顶层B+树,比较特殊,称为root bucket,其所有叶子节点保存的都是子bucket B+树根的page id.
2. 其他B+树,不妨称之为data bucket,其叶子节点可能是正常用户数据,也可能是子bucket B+树根的page id.


node/page转换:
page和node的对应关系为: 文件系统中一组连续的物理page,加载到内存成为一个逻辑page,进而转化为一个node.

subbucket:
一个bucket就是一颗完整的树,其中node都是节点(可理解为内存数据),page与node可以互相转化(可以理解page是磁盘数据).
subbucket就是某个叶子节点/分支节点的kv又指向到一个bucket.该bucket下又有许多的分支节点和叶子节点.

inline bucket:
当subbucket的数据很小时,就直接把该subbucket放到父bucket的leaf node上了.
因为每个subbucket都会占据至少一个page,若subbucket数据很少,又去重新开辟page的话就会很浪费磁盘空间.  
> inline bucket的限制: 1.没有subbucket；   2.整个bucket的大小不能超过page size/4 

Tx: 

写事务流程:
* 根据db初始化事务：拷贝一份metadata，初始化root bucket，自增txid
* 从root bucket开始，遍历B+树进行操作，所有的修改在内存中进行
* 提交写事务
  * 平衡B+树，在分裂的时候会给每个修改过的node分配新的page
  * 给freelist分配新的page
  * 将B+树数据和freelist数据写入文件
  * 将metadata写入文件

读事务流程：
* 根据db初始化事务：拷贝一份metadata，初始化root bucket
* 将当前读事务添加到db.txs中
* 从root bucket开始，遍历B+树进行查找
* 结束时，将自身移出db.txs



      






